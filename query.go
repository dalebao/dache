package localcache

import (
	"cmp"
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// Op is a comparison operator.
type Op int

const (
	OpEQ  Op = iota // equal
	OpGT            // greater than
	OpGTE           // greater than or equal
	OpLT            // less than
	OpLTE           // less than or equal
	OpIn            // in a set of values
)

// Order is a sort direction.
type Order int

const (
	Asc  Order = iota // ascending
	Desc              // descending
)

type whereClause struct {
	field string
	op    Op
	value any
}

type orderClause struct {
	field string
	order Order
}

// Query builds and executes queries against a Cache.
type Query[K comparable, V any] struct {
	cache  *Cache[K, V]
	wheres []whereClause
	orders []orderClause
	limit  int
	offset int
}

func (q *Query[K, V]) clone() *Query[K, V] {
	return &Query[K, V]{
		cache:  q.cache,
		wheres: append([]whereClause(nil), q.wheres...),
		orders: append([]orderClause(nil), q.orders...),
		limit:  q.limit,
		offset: q.offset,
	}
}

// Where adds a filter condition.
// Supported operators: OpEQ, OpGT, OpGTE, OpLT, OpLTE, OpIn.
func (q *Query[K, V]) Where(field string, op Op, val any) *Query[K, V] {
	qc := q.clone()
	qc.wheres = append(qc.wheres, whereClause{field: field, op: op, value: val})
	return qc
}

// OrderBy sets the sort order.
func (q *Query[K, V]) OrderBy(field string, order Order) *Query[K, V] {
	qc := q.clone()
	qc.orders = append(qc.orders, orderClause{field: field, order: order})
	return qc
}

// Limit sets the maximum number of results.
func (q *Query[K, V]) Limit(n int) *Query[K, V] {
	qc := q.clone()
	qc.limit = n
	return qc
}

// Offset sets the number of results to skip.
func (q *Query[K, V]) Offset(n int) *Query[K, V] {
	qc := q.clone()
	qc.offset = n
	return qc
}

// Execute runs the query and returns matching results.
func (q *Query[K, V]) Execute(ctx context.Context) ([]V, error) {
	q.cache.mu.RLock()
	defer q.cache.mu.RUnlock()

	candidates, indexField := q.collectCandidates()
	candidates = q.applyFilters(candidates, indexField)
	q.applySort(candidates)

	if q.offset > 0 && q.offset < len(candidates) {
		candidates = candidates[q.offset:]
	} else if q.offset >= len(candidates) {
		candidates = nil
	}

	if q.limit > 0 && q.limit < len(candidates) {
		candidates = candidates[:q.limit]
	}

	result := make([]V, len(candidates))
	for i, v := range candidates {
		result[i] = *v
	}
	return result, nil
}

func (q *Query[K, V]) collectCandidates() ([]*V, string) {
	var bestField string
	var bestCount int

	for _, w := range q.wheres {
		if w.op != OpEQ || w.value == nil {
			continue
		}
		idx, ok := q.cache.indices[w.field]
		if !ok {
			continue
		}
		count := len(idx.lookup(w.value))
		if bestField == "" || count < bestCount {
			bestField = w.field
			bestCount = count
		}
	}

	if bestField == "" {
		return q.allRecords(), ""
	}

	for _, w := range q.wheres {
		if w.field == bestField {
			raw := q.cache.indices[bestField].lookup(w.value)
			if len(raw) == 0 {
				return nil, bestField
			}
			cp := make([]*V, len(raw))
			copy(cp, raw)
			return cp, bestField
		}
	}

	return q.allRecords(), ""
}

func (q *Query[K, V]) allRecords() []*V {
	result := make([]*V, 0, len(q.cache.items))
	for _, v := range q.cache.items {
		result = append(result, v)
	}
	return result
}

func (q *Query[K, V]) applyFilters(candidates []*V, skipField string) []*V {
	if len(q.wheres) == 0 {
		return candidates
	}

	filtered := make([]*V, 0, len(candidates))
	for _, v := range candidates {
		match := true
		for _, w := range q.wheres {
			if w.field == skipField && w.op == OpEQ && w.value != nil {
				continue
			}
			if !q.matches(w, *v) {
				match = false
				break
			}
		}
		if match {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

func (q *Query[K, V]) matches(w whereClause, v V) bool {
	fv := fieldValue(v, w.field)
	if fv == nil {
		return false
	}
	return compare(fv, w.op, w.value)
}

func (q *Query[K, V]) applySort(candidates []*V) {
	if len(q.orders) == 0 {
		return
	}

	slices.SortFunc(candidates, func(a, b *V) int {
		for _, o := range q.orders {
			va := fieldValue(*a, o.field)
			vb := fieldValue(*b, o.field)
			c := cmpAny(va, vb)
			if c != 0 {
				if o.order == Desc {
					return -c
				}
				return c
			}
		}
		return 0
	})
}

func fieldValue(v any, field string) any {
	if m, ok := v.(fieldGetter); ok {
		return m.GetField(field)
	}
	return reflectFieldValue(v, field)
}

type fieldGetter interface {
	GetField(name string) any
}

func reflectFieldValue(v any, field string) any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	f := rv.FieldByName(field)
	if !f.IsValid() {
		return nil
	}
	return f.Interface()
}

func compare(a any, op Op, b any) bool {
	switch op {
	case OpEQ:
		return cmpAny(a, b) == 0
	case OpGT:
		return cmpAny(a, b) > 0
	case OpGTE:
		return cmpAny(a, b) >= 0
	case OpLT:
		return cmpAny(a, b) < 0
	case OpLTE:
		return cmpAny(a, b) <= 0
	case OpIn:
		return inSet(a, b)
	}
	return false
}

func inSet(a any, set any) bool {
	sv := reflect.ValueOf(set)
	if sv.Kind() != reflect.Slice && sv.Kind() != reflect.Array {
		return false
	}
	for i := 0; i < sv.Len(); i++ {
		if cmpAny(a, sv.Index(i).Interface()) == 0 {
			return true
		}
	}
	return false
}

func cmpAny(a, b any) int {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		if af < bf {
			return -1
		}
		if af > bf {
			return 1
		}
		return 0
	}

	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		return cmp.Compare(as, bs)
	}

	return strings.Compare(fmt.Sprintf("%v", a), fmt.Sprintf("%v", b))
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}
