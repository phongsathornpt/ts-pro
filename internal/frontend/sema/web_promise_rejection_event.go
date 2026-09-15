package sema

import "github.com/phongsathornpt/ts-pro/internal/core/types"

func (c *Checker) builtinPromiseRejectionEventType() *types.ObjectType {
	t := c.copyEventSemanticType("$PromiseRejectionEvent")
	t.AddField("promise", types.TypeAny, false)
	t.AddField("reason", types.TypeAny, false)
	return t
}
