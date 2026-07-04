// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// The size model below (gcSizes, optimalOrder, align, ptrdata) is adapted
// from golang.org/x/tools/go/analysis/passes/fieldalignment. The only
// behavioral change is that optimalOrder breaks ties by original field
// index (a stable order), so that fields declared together — including
// multi-name groups like "a, b int" — stay together in the rewritten
// struct. Tie-breaking never changes the computed sizes.

package fieldalign

import (
	"go/types"
	"sort"
)

// gcSizes computes sizes and alignments the way the gc compiler does,
// based on go/types.StdSizes.
type gcSizes struct {
	WordSize int64
	MaxAlign int64
}

var unsafePointerTyp = types.Unsafe.Scope().Lookup("Pointer").(*types.TypeName).Type()

func sizesFor(std types.Sizes) *gcSizes {
	return &gcSizes{
		WordSize: std.Sizeof(unsafePointerTyp),
		MaxAlign: std.Alignof(unsafePointerTyp),
	}
}

// optimalOrder returns the optimal order of struct fields, and the
// corresponding index permutation: indexes[i] is the original index of
// the field that should be placed at position i.
func optimalOrder(str *types.Struct, s *gcSizes) (*types.Struct, []int) {
	nf := str.NumFields()

	type elem struct {
		index   int
		alignof int64
		sizeof  int64
		ptrdata int64
	}

	elems := make([]elem, nf)
	for i := range nf {
		field := str.Field(i)
		ft := field.Type()
		elems[i] = elem{
			i,
			s.Alignof(ft),
			s.Sizeof(ft),
			s.ptrdata(ft),
		}
	}

	sort.Slice(elems, func(i, j int) bool {
		ei := &elems[i]
		ej := &elems[j]

		// Place zero sized objects before non-zero sized objects.
		zeroi := ei.sizeof == 0
		zeroj := ej.sizeof == 0
		if zeroi != zeroj {
			return zeroi
		}

		// Next, place more tightly aligned objects before less tightly aligned objects.
		if ei.alignof != ej.alignof {
			return ei.alignof > ej.alignof
		}

		// Place pointerful objects before pointer-free objects.
		noptrsi := ei.ptrdata == 0
		noptrsj := ej.ptrdata == 0
		if noptrsi != noptrsj {
			return noptrsj
		}

		if !noptrsi {
			// If both have pointers, place objects with less trailing
			// non-pointer bytes earlier: the field with the most trailing
			// non-pointer bytes goes at the end of the pointerful section.
			traili := ei.sizeof - ei.ptrdata
			trailj := ej.sizeof - ej.ptrdata
			if traili != trailj {
				return traili < trailj
			}
		}

		// Order by size.
		if ei.sizeof != ej.sizeof {
			return ei.sizeof > ej.sizeof
		}

		// Finally, break ties by original index so the sort is
		// deterministic and declaration groups stay contiguous.
		return ei.index < ej.index
	})

	fields := make([]*types.Var, nf)
	indexes := make([]int, nf)
	for i, e := range elems {
		fields[i] = str.Field(e.index)
		indexes[i] = e.index
	}
	return types.NewStruct(fields, nil), indexes
}

func (s *gcSizes) Alignof(T types.Type) int64 {
	// For arrays and structs, alignment is defined in terms
	// of alignment of the elements and fields, respectively.
	switch t := T.Underlying().(type) {
	case *types.Array:
		// spec: "For a variable x of array type: unsafe.Alignof(x)
		// is the same as unsafe.Alignof(x[0]), but at least 1."
		return s.Alignof(t.Elem())
	case *types.Struct:
		// spec: "For a variable x of struct type: unsafe.Alignof(x)
		// is the largest of the values unsafe.Alignof(x.f) for each
		// field f of x, but at least 1."
		max := int64(1)
		for i, nf := 0, t.NumFields(); i < nf; i++ {
			if a := s.Alignof(t.Field(i).Type()); a > max {
				max = a
			}
		}
		return max
	}
	a := s.Sizeof(T) // may be 0
	// spec: "For a variable x of any type: unsafe.Alignof(x) is at least 1."
	if a < 1 {
		return 1
	}
	if a > s.MaxAlign {
		return s.MaxAlign
	}
	return a
}

var basicSizes = [...]byte{
	types.Bool:       1,
	types.Int8:       1,
	types.Int16:      2,
	types.Int32:      4,
	types.Int64:      8,
	types.Uint8:      1,
	types.Uint16:     2,
	types.Uint32:     4,
	types.Uint64:     8,
	types.Float32:    4,
	types.Float64:    8,
	types.Complex64:  8,
	types.Complex128: 16,
}

func (s *gcSizes) Sizeof(T types.Type) int64 {
	switch t := T.Underlying().(type) {
	case *types.Basic:
		k := t.Kind()
		if int(k) < len(basicSizes) {
			if s := basicSizes[k]; s > 0 {
				return int64(s)
			}
		}
		if k == types.String {
			return s.WordSize * 2
		}
	case *types.Array:
		return t.Len() * s.Sizeof(t.Elem())
	case *types.Slice:
		return s.WordSize * 3
	case *types.Struct:
		nf := t.NumFields()
		if nf == 0 {
			return 0
		}

		var o int64
		max := int64(1)
		for i := range nf {
			ft := t.Field(i).Type()
			a, sz := s.Alignof(ft), s.Sizeof(ft)
			if a > max {
				max = a
			}
			if i == nf-1 && sz == 0 && o != 0 {
				sz = 1
			}
			o = align(o, a) + sz
		}
		return align(o, max)
	case *types.Interface:
		return s.WordSize * 2
	}
	return s.WordSize // catch-all
}

// align returns the smallest y >= x such that y % a == 0.
func align(x, a int64) int64 {
	y := x + a - 1
	return y - y%a
}

// ptrdata returns the length in bytes of the prefix of T containing
// pointers: bytes the garbage collector must scan.
func (s *gcSizes) ptrdata(T types.Type) int64 {
	switch t := T.Underlying().(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.String, types.UnsafePointer:
			return s.WordSize
		}
		return 0
	case *types.Chan, *types.Map, *types.Pointer, *types.Signature, *types.Slice:
		return s.WordSize
	case *types.Interface:
		return 2 * s.WordSize
	case *types.Array:
		n := t.Len()
		if n == 0 {
			return 0
		}
		a := s.ptrdata(t.Elem())
		if a == 0 {
			return 0
		}
		z := s.Sizeof(t.Elem())
		return (n-1)*z + a
	case *types.Struct:
		nf := t.NumFields()
		if nf == 0 {
			return 0
		}

		var o, p int64
		for i := range nf {
			ft := t.Field(i).Type()
			a, sz := s.Alignof(ft), s.Sizeof(ft)
			fp := s.ptrdata(ft)
			o = align(o, a)
			if fp != 0 {
				p = o + fp
			}
			o += sz
		}
		return p
	}

	panic("impossible")
}
