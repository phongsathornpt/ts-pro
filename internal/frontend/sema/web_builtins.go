package sema

import (
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (c *Checker) builtinDOMExceptionType() *types.ObjectType {
	if c.result.DOMExceptionType == nil {
		t := types.NewObject("$DOMException")
		t.AddField("code", types.TypeNumber, false)
		t.AddField("message", types.TypeString, false)
		t.AddField("name", types.TypeString, false)
		c.result.DOMExceptionType = t
	}
	return c.result.DOMExceptionType
}

func (c *Checker) builtinEventType() *types.ObjectType {
	if c.result.EventType == nil {
		e := types.NewObject("$Event")
		e.AddField("bubbles", types.TypeBoolean, false)
		e.AddField("cancelable", types.TypeBoolean, false)
		e.AddField("composed", types.TypeBoolean, false)
		e.AddField("currentTarget", types.TypeAny, false)
		e.AddField("defaultPrevented", types.TypeBoolean, false)
		e.AddField("eventPhase", types.TypeNumber, false)
		e.AddField("isTrusted", types.TypeBoolean, false)
		e.AddField("target", types.TypeAny, false)
		e.AddField("timeStamp", types.TypeNumber, false)
		e.AddField("type", types.TypeString, false)
		// Internal dispatch state lives in the physical object but is deliberately
		// hidden by lookupMemberType for the public Event surface.
		e.AddField("$dispatching", types.TypeBoolean, false)
		e.AddField("$inPassiveListener", types.TypeBoolean, false)
		e.AddField("$stopImmediate", types.TypeBoolean, false)
		e.AddField("$stopPropagation", types.TypeBoolean, false)
		// Variant payload slots keep one physical Event ABI for all DOM event types.
		e.AddField("$detail", types.TypeAny, false)
		e.AddField("$data", types.TypeAny, false)
		e.AddField("$origin", types.TypeString, false)
		e.AddField("$lastEventId", types.TypeString, false)
		e.AddField("$source", types.TypeAny, false)
		e.AddField("$ports", types.TypeAny, false)
		e.AddField("$message", types.TypeString, false)
		e.AddField("$filename", types.TypeString, false)
		e.AddField("$lineno", types.TypeNumber, false)
		e.AddField("$colno", types.TypeNumber, false)
		e.AddField("$error", types.TypeAny, false)
		c.result.EventType = e
	}
	return c.result.EventType
}

func (c *Checker) copyEventSemanticType(name string) *types.ObjectType {
	base := c.builtinEventType()
	t := types.NewObject(name)
	for _, fieldName := range base.FieldOrder {
		f := base.Fields[fieldName]
		t.AddField(fieldName, f.Type, f.Optional)
	}
	return t
}

func (c *Checker) builtinCustomEventType() *types.ObjectType {
	if c.result.CustomEventType == nil {
		c.result.CustomEventType = c.copyEventSemanticType("$CustomEvent")
	}
	return c.result.CustomEventType
}

func (c *Checker) builtinMessageEventType() *types.ObjectType {
	if c.result.MessageEventType == nil {
		c.result.MessageEventType = c.copyEventSemanticType("$MessageEvent")
	}
	return c.result.MessageEventType
}

func (c *Checker) builtinErrorEventType() *types.ObjectType {
	if c.result.ErrorEventType == nil {
		c.result.ErrorEventType = c.copyEventSemanticType("$ErrorEvent")
	}
	return c.result.ErrorEventType
}

func (c *Checker) builtinEventTargetType() *types.ObjectType {
	if c.result.EventTargetType == nil {
		c.result.EventTargetType = types.NewObject("$EventTarget")
	}
	return c.result.EventTargetType
}

func (c *Checker) builtinByteBufferType() *types.ObjectType {
	if c.result.ByteBufferType == nil {
		c.result.ByteBufferType = types.NewObject("$ByteBuffer")
	}
	return c.result.ByteBufferType
}

func (c *Checker) builtinArrayBufferType() *types.ObjectType {
	if c.result.ArrayBufferType == nil {
		t := types.NewObject("$ArrayBuffer")
		t.AddField("$data", c.builtinByteBufferType(), false)
		c.result.ArrayBufferType = t
	}
	return c.result.ArrayBufferType
}

func (c *Checker) builtinUint8ArrayType() *types.ObjectType {
	if c.result.Uint8ArrayType == nil {
		t := types.NewObject("$Uint8Array")
		t.AddField("$data", c.builtinByteBufferType(), false)
		t.AddField("buffer", c.builtinArrayBufferType(), false)
		t.AddField("byteOffset", types.TypeNumber, false)
		t.AddField("byteLength", types.TypeNumber, false)
		t.AddField("length", types.TypeNumber, false)
		c.result.Uint8ArrayType = t
	}
	return c.result.Uint8ArrayType
}

func (c *Checker) builtinArrayBufferMember(property string) (types.Type, bool) {
	switch property {
	case "byteLength":
		return types.TypeNumber, true
	case "slice":
		return types.NewFunction([]types.Param{{Name: "begin", Type: types.TypeNumber}, {Name: "end", Type: types.TypeNumber, Optional: true}}, c.builtinArrayBufferType()), true
	}
	return nil, false
}

func (c *Checker) builtinUint8ArrayMember(property string) (types.Type, bool) {
	switch property {
	case "length", "byteLength", "byteOffset":
		return types.TypeNumber, true
	case "buffer":
		return c.builtinArrayBufferType(), true
	case "slice", "subarray":
		return types.NewFunction([]types.Param{{Name: "begin", Type: types.TypeNumber}, {Name: "end", Type: types.TypeNumber, Optional: true}}, c.builtinUint8ArrayType()), true
	}
	return nil, false
}

func (c *Checker) builtinTextEncoderType() *types.ObjectType {
	if c.result.TextEncoderType == nil {
		c.result.TextEncoderType = types.NewObject("$TextEncoder")
	}
	return c.result.TextEncoderType
}

func (c *Checker) builtinTextDecoderType() *types.ObjectType {
	if c.result.TextDecoderType == nil {
		t := types.NewObject("$TextDecoder")
		t.AddField("fatal", types.TypeBoolean, false)
		t.AddField("ignoreBOM", types.TypeBoolean, false)
		c.result.TextDecoderType = t
	}
	return c.result.TextDecoderType
}

func (c *Checker) builtinTextEncoderMember(property string) (types.Type, bool) {
	switch property {
	case "encoding":
		return types.TypeString, true
	case "encode":
		return types.NewFunction([]types.Param{{Name: "input", Type: types.TypeString, Optional: true}}, c.builtinUint8ArrayType()), true
	case "encodeInto":
		result := types.NewObject("$TextEncoderEncodeIntoResult")
		result.AddField("read", types.TypeNumber, false)
		result.AddField("written", types.TypeNumber, false)
		return types.NewFunction([]types.Param{{Name: "source", Type: types.TypeString}, {Name: "destination", Type: c.builtinUint8ArrayType()}}, result), true
	}
	return nil, false
}

func (c *Checker) builtinTextDecoderMember(property string) (types.Type, bool) {
	switch property {
	case "encoding":
		return types.TypeString, true
	case "fatal", "ignoreBOM":
		return types.TypeBoolean, true
	case "decode":
		return types.NewFunction([]types.Param{{Name: "input", Type: c.builtinUint8ArrayType(), Optional: true}, {Name: "options", Type: types.TypeAny, Optional: true}}, types.TypeString), true
	}
	return nil, false
}

func (c *Checker) builtinURLSearchParamsType() *types.ObjectType {
	if c.result.URLSearchParamsType == nil {
		t := types.NewObject("$URLSearchParams")
		c.result.URLSearchParamsType = t
		t.AddField("$entries", types.NewArray(types.TypeString), false)
		t.AddField("$url", c.builtinURLType(), false)
	}
	return c.result.URLSearchParamsType
}

func (c *Checker) builtinURLSearchParamsMember(property string) (types.Type, bool) {
	switch property {
	case "size":
		return types.TypeNumber, true
	case "append":
		return types.NewFunction([]types.Param{{Name: "name", Type: types.TypeString}, {Name: "value", Type: types.TypeString}}, types.TypeVoid), true
	case "get":
		return types.NewFunction([]types.Param{{Name: "name", Type: types.TypeString}}, types.NewUnion(types.TypeString, types.TypeNull)), true
	case "getAll":
		return types.NewFunction([]types.Param{{Name: "name", Type: types.TypeString}}, types.NewArray(types.TypeString)), true
	case "has":
		return types.NewFunction([]types.Param{{Name: "name", Type: types.TypeString}}, types.TypeBoolean), true
	case "delete":
		return types.NewFunction([]types.Param{{Name: "name", Type: types.TypeString}}, types.TypeVoid), true
	case "set":
		return types.NewFunction([]types.Param{{Name: "name", Type: types.TypeString}, {Name: "value", Type: types.TypeString}}, types.TypeVoid), true
	case "toString":
		return types.NewFunction(nil, types.TypeString), true
	case "sort":
		return types.NewFunction(nil, types.TypeVoid), true
	}
	return nil, false
}

func (c *Checker) builtinEventMember(property string) (types.Type, bool) {
	switch property {
	case "type":
		return types.TypeString, true
	case "target", "currentTarget":
		return types.TypeAny, true
	case "bubbles", "cancelable", "defaultPrevented", "composed", "isTrusted":
		return types.TypeBoolean, true
	case "eventPhase", "timeStamp":
		return types.TypeNumber, true
	case "preventDefault", "stopPropagation", "stopImmediatePropagation":
		return types.NewFunction(nil, types.TypeVoid), true
	case "composedPath":
		return types.NewFunction(nil, types.NewArray(types.TypeAny)), true
	}
	return nil, false
}

func (c *Checker) builtinEventTargetMember(property string) (types.Type, bool) {
	event := c.builtinEventType()
	listener := types.NewFunction([]types.Param{{Name: "event", Type: event}}, types.TypeVoid)
	switch property {
	case "addEventListener":
		return types.NewFunction([]types.Param{{Name: "type", Type: types.TypeString}, {Name: "callback", Type: listener}, {Name: "options", Type: types.TypeAny, Optional: true}}, types.TypeVoid), true
	case "removeEventListener":
		return types.NewFunction([]types.Param{{Name: "type", Type: types.TypeString}, {Name: "callback", Type: listener}, {Name: "options", Type: types.TypeAny, Optional: true}}, types.TypeVoid), true
	case "dispatchEvent":
		return types.NewFunction([]types.Param{{Name: "event", Type: event}}, types.TypeBoolean), true
	}
	return nil, false
}

func (c *Checker) builtinAbortSignalType() *types.ObjectType {
	if c.result.AbortSignalType == nil {
		c.result.AbortSignalType = types.NewObject("$AbortSignal")
	}
	return c.result.AbortSignalType
}

func (c *Checker) builtinAbortControllerType() *types.ObjectType {
	if c.result.AbortControllerType == nil {
		t := types.NewObject("$AbortController")
		t.AddField("signal", c.builtinAbortSignalType(), false)
		c.result.AbortControllerType = t
	}
	return c.result.AbortControllerType
}

func (c *Checker) builtinAbortSignalMember(property string) (types.Type, bool) {
	if member, ok := c.builtinEventTargetMember(property); ok {
		return member, true
	}
	switch property {
	case "aborted":
		return types.TypeBoolean, true
	case "reason", "onabort":
		return types.TypeAny, true
	case "throwIfAborted":
		return types.NewFunction(nil, types.TypeVoid), true
	}
	return nil, false
}

func (c *Checker) builtinAbortControllerMember(property string) (types.Type, bool) {
	switch property {
	case "signal":
		return c.builtinAbortSignalType(), true
	case "abort":
		return types.NewFunction([]types.Param{{Name: "reason", Type: types.TypeAny, Optional: true}}, types.TypeVoid), true
	}
	return nil, false
}

func (c *Checker) builtinAbortSignalStaticMember(property string) (types.Type, bool) {
	signal := c.builtinAbortSignalType()
	switch property {
	case "abort":
		return types.NewFunction([]types.Param{{Name: "reason", Type: types.TypeAny, Optional: true}}, signal), true
	case "timeout":
		return types.NewFunction([]types.Param{{Name: "milliseconds", Type: types.TypeNumber}}, signal), true
	case "any":
		return types.NewFunction([]types.Param{{Name: "signals", Type: types.NewArray(signal)}}, signal), true
	}
	return nil, false
}

func (c *Checker) builtinURLType() *types.ObjectType {
	if c.result.URLType == nil {
		t := types.NewObject("$URL")
		c.result.URLType = t
		for _, name := range []string{"$scheme", "$hostname", "$port", "$pathname", "$query", "$fragment"} {
			t.AddField(name, types.TypeString, false)
		}
		t.AddField("$searchParams", c.builtinURLSearchParamsType(), false)
	}
	return c.result.URLType
}

func (c *Checker) builtinURLMember(property string) (types.Type, bool) {
	switch property {
	case "href", "origin", "protocol", "host", "hostname", "port", "pathname", "search", "hash":
		return types.TypeString, true
	case "searchParams":
		return c.builtinURLSearchParamsType(), true
	case "toString", "toJSON":
		return types.NewFunction(nil, types.TypeString), true
	}
	return nil, false
}

func (c *Checker) builtinURLStaticMember(property string) (types.Type, bool) {
	urlType := c.builtinURLType()
	baseType := types.NewUnion(types.TypeString, urlType)
	switch property {
	case "canParse":
		return types.NewFunction([]types.Param{
			{Name: "url", Type: types.TypeString},
			{Name: "base", Type: baseType, Optional: true},
		}, types.TypeBoolean), true
	case "parse":
		return types.NewFunction([]types.Param{
			{Name: "url", Type: types.TypeString},
			{Name: "base", Type: baseType, Optional: true},
		}, types.NewUnion(urlType, types.TypeNull)), true
	}
	return nil, false
}
