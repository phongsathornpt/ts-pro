package sema

import (
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (c *Checker) builtinRegExpType() *types.ObjectType {
	if c.result.RegExpType == nil {
		c.result.RegExpType = types.NewObject("$RegExp")
	}
	return c.result.RegExpType
}

func (c *Checker) builtinDateType() *types.ObjectType {
	if c.result.DateType == nil {
		c.result.DateType = types.NewObject("$Date")
	}
	return c.result.DateType
}

func (c *Checker) builtinDateMember(property string) (types.Type, bool) {
	switch property {
	case "toISOString":
		return types.NewFunction(nil, types.TypeString), true
	case "getUTCFullYear", "getUTCMonth", "getUTCDate", "getUTCHours", "getUTCMinutes", "getUTCSeconds":
		return types.NewFunction(nil, types.TypeNumber), true
	}
	return nil, false
}

func (c *Checker) builtinCollection(kind string, key, value types.Type) *BuiltinCollectionInfo {
	name := "$" + kind + "<" + key.String()
	if kind == "Map" {
		name += "," + value.String()
	}
	name += ">"
	if info := c.result.BuiltinCollections[name]; info != nil {
		return info
	}
	info := &BuiltinCollectionInfo{Kind: kind, Key: key, Value: value}
	info.Instance = types.NewObject(name)
	c.result.BuiltinCollections[name] = info
	return info
}

func (c *Checker) builtinCollectionMember(info *BuiltinCollectionInfo, property string) (types.Type, bool) {
	if property == "size" {
		return types.TypeNumber, true
	}
	keyParam := types.Param{Name: "key", Type: info.Key}
	switch info.Kind {
	case "Map":
		switch property {
		case "set":
			return types.NewFunction([]types.Param{keyParam, types.Param{Name: "value", Type: info.Value}}, info.Instance), true
		case "get":
			return types.NewFunction([]types.Param{keyParam}, types.NewUnion(info.Value, types.TypeUndefined)), true
		case "has", "delete":
			return types.NewFunction([]types.Param{keyParam}, types.TypeBoolean), true
		case "clear":
			return types.NewFunction(nil, types.TypeVoid), true
		}
	case "Set":
		switch property {
		case "add":
			return types.NewFunction([]types.Param{keyParam}, info.Instance), true
		case "has", "delete":
			return types.NewFunction([]types.Param{keyParam}, types.TypeBoolean), true
		case "clear":
			return types.NewFunction(nil, types.TypeVoid), true
		}
	}
	return nil, false
}
