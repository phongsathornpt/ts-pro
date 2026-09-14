package sema

import "github.com/phongsathornpt/ts-pro/internal/core/types"

func (c *Checker) builtinPerformanceType() *types.ObjectType {
	obj := types.NewObject("$Performance")
	json := types.NewObject("$PerformanceJSON")
	json.AddField("timeOrigin", types.TypeNumber, false)

	obj.AddField("now", types.NewFunction(nil, types.TypeNumber), false)
	obj.AddField("timeOrigin", types.TypeNumber, false)
	obj.AddField("toJSON", types.NewFunction(nil, json), false)

	for _, property := range []string{"addEventListener", "removeEventListener", "dispatchEvent"} {
		member, ok := c.builtinEventTargetMember(property)
		if ok {
			obj.AddField(property, member, false)
		}
	}
	return obj
}
