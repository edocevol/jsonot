package sharedb

// Redis adapter for ShareDB
//
// Usage
// -----
//   rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//   backend := sharedb.NewRedisBackend(rdb)
//   locker  := sharedb.NewRedisLocker(rdb, sharedb.DefaultRedisLockTTL)
//   pub     := sharedb.NewRedisPublisher(rdb)
//   server  := sharedb.NewServer(backend, locker, sharedb.WithPublisher(pub))
//
// This requires the go-redis/v9 module:
//   go get github.com/redis/go-redis/v9
//
// Key layout
// ----------
//   sharedb:{docID}:snap         HASH  { version, doc }
//   sharedb:{docID}:ops          LIST  of JSON-encoded OpRecord, index 0 = version 1
//   sharedb:{docID}:lock         STRING  ephemeral lock token (NX, TTL)
//   sharedb:events               CHANNEL  pub/sub channel for all events
//
// Centralized version number
// --------------------------
// The version number lives inside the snapshot HASH field "version" and is
// incremented by the Server under the per-document RedisLocker. The locker
// uses SET NX with a TTL so no two Server instances can commit to the same
// document at the same time. This guarantees serial version progression
// globally, enabling correct OT transforms across horizontal replicas.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// DefaultRedisLockTTL is the TTL applied to per-document Redis locks.
// Increase it if Submit is expected to take longer (e.g. large transforms).
const DefaultRedisLockTTL = 5 * time.Second

// ─── Key helpers ───────────────────────────────────────────────────────────────

func redisSnapKey(docID string) string { return "sharedb:" + docID + ":snap" }
func redisOpsKey(docID string) string  { return "sharedb:" + docID + ":ops" }
func redisLockKey(docID string) string { return "sharedb:" + docID + ":lock" }

const redisEventsChannel = "sharedb:events"

// ─── RedisBackend ──────────────────────────────────────────────────────────────

// RedisBackend implements Backend using Redis.
//
// Snapshot is stored as a Redis HASH keyed by sharedb:{docID}:snap.
// Op log is stored as a Redis LIST keyed by sharedb:{docID}:ops.
type RedisBackend struct {
	rdb *redis.Client
}

// NewRedisBackend creates a RedisBackend that uses rdb.
func NewRedisBackend(rdb *redis.Client) *RedisBackend {
	return &RedisBackend{rdb: rdb}
}

var _ Backend = (*RedisBackend)(nil)

func (b *RedisBackend) CreateDoc(ctx context.Context, docID string, initial json.RawMessage) error {
	snapKey := redisSnapKey(docID)

	// Use HSETNX on "version" to guarantee atomicity — if the key already has
	// a version field the document exists.
	set, err := b.rdb.HSetNX(ctx, snapKey, "version", 0).Result()
	if err != nil {
		return err
	}
	if !set {
		return ErrDocumentExists
	}

	payload := initial
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	return b.rdb.HSet(ctx, snapKey, "doc", string(payload)).Err()
}

func (b *RedisBackend) GetDoc(ctx context.Context, docID string) (DocRecord, error) {
	vals, err := b.rdb.HGetAll(ctx, redisSnapKey(docID)).Result()
	if err != nil {
		return DocRecord{}, err
	}
	if len(vals) == 0 {
		return DocRecord{}, ErrDocumentNotFound
	}

	version, err := strconv.Atoi(vals["version"])
	if err != nil {
		return DocRecord{}, fmt.Errorf("sharedb redis: corrupt version field for %s: %w", docID, err)
	}

	return DocRecord{
		DocumentID: docID,
		Version:    version,
		Doc:        json.RawMessage(vals["doc"]),
	}, nil
}

func (b *RedisBackend) SaveDoc(ctx context.Context, record DocRecord) error {
	snapKey := redisSnapKey(record.DocumentID)
	return b.rdb.HSet(ctx, snapKey,
		"version", record.Version,
		"doc", string(record.Doc),
	).Err()
}

func (b *RedisBackend) AppendOp(ctx context.Context, record OpRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return b.rdb.RPush(ctx, redisOpsKey(record.DocumentID), raw).Err()
}

// GetOps returns ops in (fromVersion, toVersion], i.e. op records whose
// Version field is in [fromVersion+1, toVersion]. The LIST index is
// 0-based where index i = version i+1, so LRANGE start=fromVersion end=toVersion-1.
func (b *RedisBackend) GetOps(ctx context.Context, docID string, fromVersion, toVersion int) ([]OpRecord, error) {
	if fromVersion == toVersion {
		return nil, nil
	}

	raws, err := b.rdb.LRange(ctx, redisOpsKey(docID), int64(fromVersion), int64(toVersion-1)).Result()
	if err != nil {
		return nil, err
	}

	records := make([]OpRecord, 0, len(raws))
	for _, raw := range raws {
		var rec OpRecord
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			return nil, fmt.Errorf("sharedb redis: failed to decode op record: %w", err)
		}
		records = append(records, rec)
	}

	return records, nil
}

// ─── RedisLocker ───────────────────────────────────────────────────────────────

// RedisLocker implements per-document mutual exclusion via Redis SET NX.
// It provides distributed locking across multiple Server instances,
// ensuring that the version number is advanced by only one node at a time.
//
// For production use, consider replacing this with Redlock
// (github.com/go-redsync/redsync) for stronger guarantees against
// clock skew and Redis failover.
type RedisLocker struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewRedisLocker creates a RedisLocker with the given lock TTL.
func NewRedisLocker(rdb *redis.Client, ttl time.Duration) *RedisLocker {
	return &RedisLocker{rdb: rdb, ttl: ttl}
}

var _ Locker = (*RedisLocker)(nil)

func (l *RedisLocker) Lock(ctx context.Context, docID string) (func(), error) {
	key := redisLockKey(docID)
	token := fmt.Sprintf("%d", time.Now().UnixNano())

	const maxRetries = 50
	const retryDelay = 50 * time.Millisecond

	for i := range maxRetries {
		set, err := l.rdb.SetNX(ctx, key, token, l.ttl).Result()
		if err != nil {
			return nil, err
		}
		if set {
			unlock := func() {
				// Only delete if token still matches (guard against TTL expiry).
				script := redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0`)
				_ = script.Run(ctx, l.rdb, []string{key}, token).Err()
			}
			return unlock, nil
		}

		if i == maxRetries-1 {
			break
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryDelay):
		}
	}

	return nil, errors.New("sharedb redis: failed to acquire lock for document " + docID)
}

// ─── RedisPublisher ────────────────────────────────────────────────────────────

// RedisPublisher implements Publisher using Redis Pub/Sub.
// Events are published to the "sharedb:events" channel and routed to
// document-specific subscribers in each process. This allows events to
// flow between horizontally scaled Server instances.
type RedisPublisher struct {
	rdb  *redis.Client
	mu   sync.Mutex
	subs map[int]*redisSub
	next int
}

type redisSub struct {
	docID string
	ch    chan Event
}

// NewRedisPublisher creates a RedisPublisher and starts the background
// listener on the shared pub/sub channel.
func NewRedisPublisher(rdb *redis.Client) *RedisPublisher {
	p := &RedisPublisher{
		rdb:  rdb,
		subs: make(map[int]*redisSub),
	}
	go p.listen()
	return p
}

var _ Publisher = (*RedisPublisher)(nil)

func (p *RedisPublisher) Publish(ctx context.Context, event Event) {
	raw, err := json.Marshal(event)
	if err != nil {
		return
	}
	_ = p.rdb.Publish(ctx, redisEventsChannel, raw).Err()
}

func (p *RedisPublisher) Subscribe(_ context.Context, docID string, buffer int) (<-chan Event, func(), error) {
	if buffer < 0 {
		buffer = 0
	}

	p.mu.Lock()
	id := p.next
	p.next++
	sub := &redisSub{docID: docID, ch: make(chan Event, buffer)}
	p.subs[id] = sub
	p.mu.Unlock()

	cancel := func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if s, ok := p.subs[id]; ok {
			delete(p.subs, id)
			close(s.ch)
		}
	}

	return sub.ch, cancel, nil
}

func (p *RedisPublisher) listen() {
	sub := p.rdb.Subscribe(context.Background(), redisEventsChannel)
	defer sub.Close()

	ch := sub.Channel()
	for msg := range ch {
		var event Event
		if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
			continue
		}

		p.mu.Lock()
		for _, s := range p.subs {
			if s.docID != event.DocumentID {
				continue
			}
			select {
			case s.ch <- event:
			default:
			}
		}
		p.mu.Unlock()
	}
}

// NewRedisServer creates a fully Redis-backed Server.
func NewRedisServer(rdb *redis.Client) *Server {
	return NewServer(
		NewRedisBackend(rdb),
		NewRedisLocker(rdb, DefaultRedisLockTTL),
		WithPublisher(NewRedisPublisher(rdb)),
	)
}
