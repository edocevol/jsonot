package jsonot

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/samber/mo"
)

// PathElement is a element of path
type PathElement struct {
	Key   string
	Index int
}

// Equals checks if two PathElement are equal
func (pe PathElement) Equals(other PathElement) bool {
	if pe.Key != "" || other.Key != "" {
		return pe.Key == other.Key
	}
	return pe.Index == other.Index
}

// PathElementFromKey create a PathElement from key
func PathElementFromKey(k string) PathElement {
	return PathElement{Key: k}
}

// PathElementFromIndex create a PathElement from index
func PathElementFromIndex(i int) PathElement {
	return PathElement{Index: i}
}

// Path is a path of json object
type Path struct {
	Paths []PathElement
}

// NewPathFromIndexes create a new Path from indexes
func NewPathFromIndexes(indexes []int) Path {
	paths := make([]PathElement, len(indexes))
	for i, index := range indexes {
		paths[i] = PathElement{Index: index}
	}
	return Path{Paths: paths}
}

// NewPathFromKeys create a new Path from keys
func NewPathFromKeys(keys []string) Path {
	paths := make([]PathElement, len(keys))
	for i, key := range keys {
		paths[i] = PathElement{Key: key}
	}
	return Path{Paths: paths}
}

// Format formats the path as a string
func (p *Path) Format(st fmt.State, verb rune) {
	var elements []string
	for _, elem := range p.Paths {
		if elem.Key != "" {
			elements = append(elements, fmt.Sprintf("%q", elem.Key))
		} else {
			elements = append(elements, strconv.Itoa(elem.Index))
		}
	}
	_, _ = fmt.Fprintf(st, "[%s]", strings.Join(elements, ", "))
}

// ToValue convert the Path to a Value
func (p *Path) ToValue() Value {
	var paths []any
	for _, pe := range p.Paths {
		if pe.Key != "" {
			paths = append(paths, pe.Key)
		} else {
			paths = append(paths, float64(pe.Index)) // JSON requires numbers to be float64
		}
	}

	return ValueFromAny(paths)
}

// FromValue create a new Path from a Value
func (p *Path) FromValue(path Value) {
	var paths []PathElement
	pathValue := path.GetArray()
	for _, v := range pathValue.MustGet() {
		pe := PathElement{}
		matched := false
		if v.IsString() {
			matched = true
			pe.Key = v.GetString().MustGet()
		}
		if v.IsInt() {
			matched = true
			pe.Index = v.GetInt().MustGet()
		}
		if matched {
			paths = append(paths, pe)
		}
	}

	p.Paths = paths
}

// Equal check if two path is equal
func (p *Path) Equal(other Path) bool {
	if len(p.Paths) != len(other.Paths) {
		return false
	}

	for i, pe := range p.Paths {
		if pe != other.Paths[i] {
			return false
		}
	}
	return true
}

// Clone create a copy of the path
func (p *Path) Clone() Path {
	np := Path{
		Paths: make([]PathElement, len(p.Paths)),
	}
	copy(np.Paths, p.Paths)
	return np
}

// FirstKeyPath get the first key of path
func (p *Path) FirstKeyPath() mo.Option[string] {
	if len(p.Paths) > 0 {
		return mo.Some(p.Paths[0].Key)
	}
	return mo.None[string]()
}

// FirstIndexPath get the first index of path
func (p *Path) FirstIndexPath() mo.Option[int] {
	if len(p.Paths) > 0 {
		if p.Paths[0].Index > 0 {
			return mo.Some(p.Paths[0].Index)
		}
		if p.Paths[0].Key != "" {
			index, err := strconv.Atoi(p.Paths[0].Key)
			if err == nil {
				return mo.Some(index)
			}
		}
		return mo.Some(0)
	}

	return mo.None[int]()
}

// Get try to get the path element at index
func (p *Path) Get(index int) mo.Option[PathElement] {
	if index < len(p.Paths) {
		return mo.Some(p.Paths[index])
	}
	return mo.None[PathElement]()
}

// GetElements get the path elements
func (p *Path) GetElements() []PathElement {
	return p.Paths
}

// GetMutElements get the path elements
func (p *Path) GetMutElements() *[]PathElement {
	return &p.Paths
}

// GetKeyAt get the key at index
func (p *Path) GetKeyAt(index int) mo.Option[string] {
	if index < len(p.Paths) {
		return mo.Some(p.Paths[index].Key)
	}
	return mo.None[string]()
}

// GetIndexAt get the index at index
func (p *Path) GetIndexAt(index int) mo.Option[int] {
	if index < len(p.Paths) {
		return mo.Some(p.Paths[index].Index)
	}
	return mo.None[int]()
}

// Last get the last path element
func (p *Path) Last() mo.Option[PathElement] {
	if len(p.Paths) > 0 {
		return mo.Some(p.Paths[len(p.Paths)-1])
	}
	return mo.None[PathElement]()
}

// Replace try to replace the path element at index
func (p *Path) Replace(index int, pathElem PathElement) mo.Option[PathElement] {
	if index < len(p.Paths) {
		old := p.Paths[index]
		p.Paths[index] = pathElem
		return mo.Some(old)
	}

	return mo.None[PathElement]()
}

// IncreaseIndex increase the index at index
func (p *Path) IncreaseIndex(index int) bool {
	if index < len(p.Paths) {
		if p.Paths[index].Index >= 0 {
			p.Paths[index].Index++
			return true
		}
	}
	return false
}

// DecreaseIndex decrease the index at index
func (p *Path) DecreaseIndex(index int) bool {
	if index < len(p.Paths) {
		if p.Paths[index].Index > 0 {
			p.Paths[index].Index--
			return true
		}
	}
	return false
}

// SplitAt split the path at index
func (p *Path) SplitAt(mid int) (left, right Path) {
	return Path{Paths: p.Paths[:mid]}, Path{Paths: p.Paths[mid:]}
}

// MaxCommonPath get the max common path
func (p *Path) MaxCommonPath(path Path) *Path {
	var commonP []PathElement
	for i, pa := range path.GetElements() {
		if pb := p.Get(i); pb.IsPresent() && pa == pb.MustGet() {
			commonP = append(commonP, pb.MustGet())
			continue
		}
		break
	}
	return &Path{Paths: commonP}
}

// CommonPathPrefix get the common path prefix
func (p *Path) CommonPathPrefix(path Path) *Path {
	var cp []PathElement
	for i, pa := range path.GetElements() {
		if pb := p.Get(i); pb.IsPresent() && pa == pb.MustGet() {
			cp = append(cp, pa)
			continue
		}
		break
	}

	return &Path{Paths: cp}
}

// IsEmpty check if the path is empty
func (p *Path) IsEmpty() bool {
	return len(p.Paths) == 0
}

// IsPrefixOf check if the path is prefix of another path
func (p *Path) IsPrefixOf(other Path) bool {
	for i, p1 := range p.Paths {
		p2 := other.Get(i)
		if p2.IsAbsent() {
			return false
		}
		if !p1.Equals(p2.MustGet()) {
			return false
		}
	}
	return true
}

// Len get the length of path
func (p *Path) Len() int {
	return len(p.Paths)
}

// NextLevel get the next level path
func (p *Path) NextLevel() Path {
	return Path{Paths: p.Paths[1:]}
}

// UnmarshalJSON unmarshal json to path
func (p *Path) UnmarshalJSON(data []byte) error {
	var paths []any
	if err := json.Unmarshal(data, &paths); err != nil {
		return err
	}

	for _, pe := range paths {
		switch pe := pe.(type) {
		case string:
			p.Paths = append(p.Paths, PathElement{Key: pe})
		case float64:
			p.Paths = append(p.Paths, PathElement{Key: strconv.Itoa(int(pe))})
		}
	}
	return nil
}

// String 实现 fmt.Stringer trait for Path
func (p *Path) String() string {
	var elements []string
	for _, elem := range p.Paths {
		if elem.Key != "" {
			elements = append(elements, "\""+elem.Key+"\"")
		} else {
			elements = append(elements, strconv.Itoa(elem.Index))
		}
	}
	return "[" + strings.Join(elements, ", ") + "]"
}
