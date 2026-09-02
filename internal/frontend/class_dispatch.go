package frontend

import "sort"

func (e *extractor) recordConcreteClass(symbol SymbolID, value *Expr) {
	if value != nil && value.ConcreteKnown {
		if class, ok := e.classesByType[value.ConcreteType]; ok {
			e.concreteClasses[symbol] = class
			return
		}
	}
	delete(e.concreteClasses, symbol)
}

func (e *extractor) findClassMethod(info *classInfo, name string) (FunctionID, bool) {
	for current := info; current != nil; current = current.Base {
		if target, ok := current.Methods[name]; ok {
			return target, true
		}
	}
	return 0, false
}

func cloneConcreteClasses(source map[SymbolID]*classInfo) map[SymbolID]*classInfo {
	result := make(map[SymbolID]*classInfo, len(source))
	for symbol, class := range source {
		result[symbol] = class
	}
	return result
}
func mergeConcreteClasses(left, right map[SymbolID]*classInfo) map[SymbolID]*classInfo {
	result := make(map[SymbolID]*classInfo)
	for symbol, class := range left {
		if other, ok := right[symbol]; ok && other == class {
			result[symbol] = class
		}
	}
	return result
}

func (e *extractor) withConcreteSnapshot(fn func() error) (map[SymbolID]*classInfo, error) {
	before := cloneConcreteClasses(e.concreteClasses)
	if err := fn(); err != nil {
		e.concreteClasses = before
		return nil, err
	}
	after := cloneConcreteClasses(e.concreteClasses)
	e.concreteClasses = before
	return after, nil
}

func isSubclassOf(candidate, base *classInfo) bool {
	for current := candidate.Base; current != nil; current = current.Base {
		if current == base {
			return true
		}
	}
	return false
}

func (e *extractor) hasKnownOverride(base *classInfo, name string, baseTarget FunctionID) bool {
	seen := map[*classInfo]struct{}{}
	for _, candidate := range e.classesByType {
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}
		if candidate == base || !isSubclassOf(candidate, base) {
			continue
		}
		if target, ok := candidate.Methods[name]; ok && target != baseTarget {
			return true
		}
	}
	return false
}

func (e *extractor) dispatchTargets(base *classInfo, name string) []DispatchTarget {
	seen := map[*classInfo]struct{}{}
	var result []DispatchTarget
	for _, candidate := range e.classesByType {
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}
		if candidate != base && !isSubclassOf(candidate, base) {
			continue
		}
		target, ok := e.findClassMethod(candidate, name)
		if !ok {
			continue
		}
		tag := e.result.Shapes[candidate.Shape].ClassTag
		result = append(result, DispatchTarget{ClassTag: tag, Function: target})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ClassTag < result[j].ClassTag })
	return result
}
func (e *extractor) dynamicMethodTargets(name string) []DispatchTarget {
	seen := map[*classInfo]struct{}{}
	var result []DispatchTarget
	for _, candidate := range e.classesByType {
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}
		target, ok := e.findClassMethod(candidate, name)
		if !ok {
			continue
		}
		tag := e.result.Shapes[candidate.Shape].ClassTag
		result = append(result, DispatchTarget{ClassTag: tag, Function: target})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ClassTag < result[j].ClassTag })
	return result
}
