package sema

import "github.com/phongsathornpt/ts-pro/internal/core/types"

func (c *Checker) builtinPromiseRejectionEventType() *types.ObjectType {
	return c.copyEventSemanticType("$PromiseRejectionEvent")
}
