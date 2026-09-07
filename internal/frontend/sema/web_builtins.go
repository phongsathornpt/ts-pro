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
		c.builtinEventType()
		c.builtinDOMExceptionType()
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

func (c *Checker) builtinURLPatternType() *types.ObjectType {
	if c.result.URLPatternType == nil {
		_ = c.builtinURLType()
		_ = c.builtinDOMExceptionType()
		t := types.NewObject("$URLPattern")
		c.result.URLPatternType = t
		for _, name := range []string{"protocol", "username", "password", "hostname", "port", "pathname", "search", "hash"} {
			t.AddField(name, types.TypeString, false)
		}
		t.AddField("hasRegExpGroups", types.TypeBoolean, false)
		for _, name := range []string{"$protocolRegex", "$usernameRegex", "$passwordRegex", "$hostnameRegex", "$portRegex", "$pathnameRegex", "$searchRegex", "$hashRegex", "$groupKeys"} {
			t.AddField(name, types.TypeString, false)
		}
	}
	return c.result.URLPatternType
}

func (c *Checker) builtinURLPatternComponentResultType() *types.ObjectType {
	if c.result.URLPatternComponentResultType == nil {
		t := types.NewObject("$URLPatternComponentResult")
		c.result.URLPatternComponentResultType = t
		t.AddField("input", types.TypeString, false)
		t.AddField("groups", types.TypeAny, false)
	}
	return c.result.URLPatternComponentResultType
}

func (c *Checker) builtinURLPatternResultType() *types.ObjectType {
	if c.result.URLPatternResultType == nil {
		t := types.NewObject("$URLPatternResult")
		c.result.URLPatternResultType = t
		comp := c.builtinURLPatternComponentResultType()
		t.AddField("inputs", types.NewArray(types.TypeAny), false)
		for _, name := range []string{"protocol", "username", "password", "hostname", "port", "pathname", "search", "hash"} {
			t.AddField(name, comp, false)
		}
	}
	return c.result.URLPatternResultType
}

func (c *Checker) builtinURLPatternInitType() *types.ObjectType {
	if c.result.URLPatternInitType == nil {
		t := types.NewObject("$URLPatternInit")
		c.result.URLPatternInitType = t
		for _, name := range []string{"protocol", "username", "password", "hostname", "port", "pathname", "search", "hash", "baseURL"} {
			t.AddField(name, types.TypeString, false)
		}
	}
	return c.result.URLPatternInitType
}

func (c *Checker) builtinURLPatternMember(property string) (types.Type, bool) {
	switch property {
	case "protocol", "username", "password", "hostname", "port", "pathname", "search", "hash":
		return types.TypeString, true
	case "hasRegExpGroups":
		return types.TypeBoolean, true
	case "test":
		return types.NewFunction([]types.Param{
			{Name: "input", Type: types.TypeAny, Optional: true},
			{Name: "baseURL", Type: types.TypeString, Optional: true},
		}, types.TypeBoolean), true
	case "exec":
		return types.NewFunction([]types.Param{
			{Name: "input", Type: types.TypeAny, Optional: true},
			{Name: "baseURL", Type: types.TypeString, Optional: true},
		}, c.builtinURLPatternResultType()), true
	}
	return nil, false
}

func (c *Checker) builtinBlobType() *types.ObjectType {
	if c.result.BlobType == nil {
		t := types.NewObject("$Blob")
		t.AddField("size", types.TypeNumber, false)
		t.AddField("type", types.TypeString, false)
		t.AddField("$data", c.builtinByteBufferType(), false)
		c.result.BlobType = t
	}
	return c.result.BlobType
}

func (c *Checker) builtinBlobMember(property string) (types.Type, bool) {
	switch property {
	case "size":
		return types.TypeNumber, true
	case "type":
		return types.TypeString, true
	case "slice":
		return types.NewFunction([]types.Param{
			{Name: "start", Type: types.TypeNumber, Optional: true},
			{Name: "end", Type: types.TypeNumber, Optional: true},
			{Name: "contentType", Type: types.TypeString, Optional: true},
		}, c.builtinBlobType()), true
	case "text":
		return types.NewFunction(nil, c.newPromiseType(types.TypeString)), true
	case "arrayBuffer":
		return types.NewFunction(nil, c.newPromiseType(c.builtinArrayBufferType())), true
	case "bytes":
		return types.NewFunction(nil, c.newPromiseType(c.builtinUint8ArrayType())), true
	case "stream":
		return types.NewFunction(nil, c.builtinReadableStreamType()), true
	}
	return nil, false
}

func (c *Checker) builtinFileType() *types.ObjectType {
	if c.result.FileType == nil {
		t := types.NewObject("$File")
		t.AddField("size", types.TypeNumber, false)
		t.AddField("type", types.TypeString, false)
		t.AddField("name", types.TypeString, false)
		t.AddField("lastModified", types.TypeNumber, false)
		t.AddField("webkitRelativePath", types.TypeString, false)
		t.AddField("$data", c.builtinByteBufferType(), false)
		c.result.FileType = t
	}
	return c.result.FileType
}

func (c *Checker) builtinFileMember(property string) (types.Type, bool) {
	switch property {
	case "name", "webkitRelativePath":
		return types.TypeString, true
	case "lastModified":
		return types.TypeNumber, true
	default:
		return c.builtinBlobMember(property)
	}
}

func (c *Checker) builtinFormDataType() *types.ObjectType {
	// FormData values may contain File entries, including values produced by Body.formData().
	// Materialize File even when user source never names it directly so IR lowering has a stable layout.
	c.builtinFileType()
	if c.result.FormDataType == nil {
		t := types.NewObject("$FormData")
		t.AddField("$entries", types.NewArray(types.TypeAny), false)
		c.result.FormDataType = t
	}
	return c.result.FormDataType
}

func (c *Checker) builtinFormDataMember(property string) (types.Type, bool) {
	switch property {
	case "append":
		return types.NewFunction([]types.Param{
			{Name: "name", Type: types.TypeString},
			{Name: "value", Type: types.TypeAny},
			{Name: "filename", Type: types.TypeString, Optional: true},
		}, types.TypeVoid), true
	case "set":
		return types.NewFunction([]types.Param{
			{Name: "name", Type: types.TypeString},
			{Name: "value", Type: types.TypeAny},
			{Name: "filename", Type: types.TypeString, Optional: true},
		}, types.TypeVoid), true
	case "delete":
		return types.NewFunction([]types.Param{
			{Name: "name", Type: types.TypeString},
		}, types.TypeVoid), true
	case "get":
		return types.NewFunction([]types.Param{
			{Name: "name", Type: types.TypeString},
		}, types.TypeAny), true
	case "getAll":
		return types.NewFunction([]types.Param{
			{Name: "name", Type: types.TypeString},
		}, types.NewArray(types.TypeAny)), true
	case "has":
		return types.NewFunction([]types.Param{
			{Name: "name", Type: types.TypeString},
		}, types.TypeBoolean), true
	case "keys":
		return types.NewFunction(nil, types.NewArray(types.TypeString)), true
	case "values":
		return types.NewFunction(nil, types.NewArray(types.TypeAny)), true
	case "entries":
		return types.NewFunction(nil, types.NewArray(types.TypeAny)), true
	case "forEach":
		cbType := types.NewFunction([]types.Param{
			{Name: "value", Type: types.TypeAny},
			{Name: "key", Type: types.TypeString},
			{Name: "parent", Type: types.TypeAny, Optional: true},
		}, types.TypeVoid)
		return types.NewFunction([]types.Param{
			{Name: "callback", Type: cbType},
		}, types.TypeVoid), true
	}
	return nil, false
}

func (c *Checker) builtinHeadersType() *types.ObjectType {
	if c.result.HeadersType == nil {
		t := types.NewObject("$Headers")
		c.result.HeadersType = t
		t.AddField("$entries", types.NewArray(types.TypeString), false)
	}
	return c.result.HeadersType
}

func (c *Checker) builtinHeadersMember(property string) (types.Type, bool) {
	switch property {
	case "append":
		return types.NewFunction([]types.Param{
			{Name: "name", Type: types.TypeString},
			{Name: "value", Type: types.TypeString},
		}, types.TypeVoid), true
	case "delete":
		return types.NewFunction([]types.Param{
			{Name: "name", Type: types.TypeString},
		}, types.TypeVoid), true
	case "get":
		return types.NewFunction([]types.Param{
			{Name: "name", Type: types.TypeString},
		}, types.NewUnion(types.TypeString, types.TypeNull)), true
	case "getSetCookie":
		return types.NewFunction(nil, types.NewArray(types.TypeString)), true
	case "has":
		return types.NewFunction([]types.Param{
			{Name: "name", Type: types.TypeString},
		}, types.TypeBoolean), true
	case "set":
		return types.NewFunction([]types.Param{
			{Name: "name", Type: types.TypeString},
			{Name: "value", Type: types.TypeString},
		}, types.TypeVoid), true
	case "forEach":
		cbType := types.NewFunction([]types.Param{
			{Name: "value", Type: types.TypeString},
			{Name: "key", Type: types.TypeString},
			{Name: "parent", Type: types.TypeAny, Optional: true},
		}, types.TypeVoid)
		return types.NewFunction([]types.Param{
			{Name: "callback", Type: cbType},
			{Name: "thisArg", Type: types.TypeAny, Optional: true},
		}, types.TypeVoid), true
	case "keys":
		return types.NewFunction(nil, types.NewArray(types.TypeString)), true
	case "values":
		return types.NewFunction(nil, types.NewArray(types.TypeString)), true
	case "entries":
		return types.NewFunction(nil, types.NewArray(types.NewArray(types.TypeString))), true
	}
	return nil, false
}

func (c *Checker) builtinRequestType() *types.ObjectType {
	// Request URL normalization reuses the URL record layout during IR lowering.
	// Materialize that builtin dependency even when source code never names URL directly.
	c.builtinURLType()
	if c.result.RequestType == nil {
		_ = c.builtinReadableStreamType()
		t := types.NewObject("$Request")
		c.result.RequestType = t
		t.AddField("$bodyData", c.builtinByteBufferType(), false)
		t.AddField("$hasBody", types.TypeBoolean, false)
		t.AddField("$bodyStream", types.TypeAny, false)
		t.AddField("bodyUsed", types.TypeBoolean, false)
		t.AddField("headers", c.builtinHeadersType(), false)
		t.AddField("method", types.TypeString, false)
		t.AddField("signal", c.builtinAbortSignalType(), false)
		t.AddField("url", types.TypeString, false)
	}
	return c.result.RequestType
}

func (c *Checker) builtinRequestMember(property string) (types.Type, bool) {
	switch property {
	case "method", "url":
		return types.TypeString, true
	case "headers":
		return c.builtinHeadersType(), true
	case "signal":
		return c.builtinAbortSignalType(), true
	case "bodyUsed":
		return types.TypeBoolean, true
	case "body":
		return types.NewUnion(c.builtinReadableStreamType(), types.TypeNull), true
	case "clone":
		return types.NewFunction(nil, c.builtinRequestType()), true
	case "text":
		return types.NewFunction(nil, c.newPromiseType(types.TypeString)), true
	case "arrayBuffer":
		return types.NewFunction(nil, c.newPromiseType(c.builtinArrayBufferType())), true
	case "bytes":
		return types.NewFunction(nil, c.newPromiseType(c.builtinUint8ArrayType())), true
	case "blob":
		return types.NewFunction(nil, c.newPromiseType(c.builtinBlobType())), true
	case "json":
		return types.NewFunction(nil, c.newPromiseType(types.TypeAny)), true
	case "formData":
		return types.NewFunction(nil, c.newPromiseType(c.builtinFormDataType())), true
	case "textStream":
		return types.NewFunction(nil, c.builtinReadableStreamType()), true
	}
	return nil, false
}

func (c *Checker) builtinResponseType() *types.ObjectType {
	if c.result.ResponseType == nil {
		_ = c.builtinReadableStreamType()
		t := types.NewObject("$Response")
		c.result.ResponseType = t
		t.AddField("$bodyData", c.builtinByteBufferType(), false)
		t.AddField("$hasBody", types.TypeBoolean, false)
		t.AddField("$bodyStream", types.TypeAny, false)
		t.AddField("bodyUsed", types.TypeBoolean, false)
		t.AddField("headers", c.builtinHeadersType(), false)
		t.AddField("ok", types.TypeBoolean, false)
		t.AddField("redirected", types.TypeBoolean, false)
		t.AddField("status", types.TypeNumber, false)
		t.AddField("statusText", types.TypeString, false)
		t.AddField("type", types.TypeString, false)
		t.AddField("url", types.TypeString, false)
	}
	return c.result.ResponseType
}

func (c *Checker) builtinResponseStaticMember(property string) (types.Type, bool) {
	switch property {
	case "error":
		return types.NewFunction(nil, c.builtinResponseType()), true
	case "redirect":
		_ = c.builtinURLType()
		return types.NewFunction([]types.Param{{Name: "url", Type: types.TypeString}, {Name: "status", Type: types.TypeNumber, Optional: true}}, c.builtinResponseType()), true
	case "json":
		return types.NewFunction([]types.Param{{Name: "data", Type: types.TypeAny}, {Name: "init", Type: types.TypeAny, Optional: true}}, c.builtinResponseType()), true
	}
	return nil, false
}

func (c *Checker) builtinResponseMember(property string) (types.Type, bool) {
	switch property {
	case "status":
		return types.TypeNumber, true
	case "statusText", "type", "url":
		return types.TypeString, true
	case "headers":
		return c.builtinHeadersType(), true
	case "bodyUsed", "ok", "redirected":
		return types.TypeBoolean, true
	case "body":
		return types.NewUnion(c.builtinReadableStreamType(), types.TypeNull), true
	case "clone":
		return types.NewFunction(nil, c.builtinResponseType()), true
	case "text":
		return types.NewFunction(nil, c.newPromiseType(types.TypeString)), true
	case "arrayBuffer":
		return types.NewFunction(nil, c.newPromiseType(c.builtinArrayBufferType())), true
	case "bytes":
		return types.NewFunction(nil, c.newPromiseType(c.builtinUint8ArrayType())), true
	case "blob":
		return types.NewFunction(nil, c.newPromiseType(c.builtinBlobType())), true
	case "json":
		return types.NewFunction(nil, c.newPromiseType(types.TypeAny)), true
	case "formData":
		return types.NewFunction(nil, c.newPromiseType(c.builtinFormDataType())), true
	}
	return nil, false
}

func (c *Checker) builtinReadableStreamReadResultType() *types.ObjectType {
	if c.result.ReadableStreamReadResultType == nil {
		t := types.NewObject("$ReadableStreamReadResult")
		t.AddField("value", types.TypeAny, false)
		t.AddField("done", types.TypeBoolean, false)
		c.result.ReadableStreamReadResultType = t
	}
	return c.result.ReadableStreamReadResultType
}

func (c *Checker) builtinReadableStreamDefaultReaderType() *types.ObjectType {
	if c.result.ReadableStreamDefaultReaderType == nil {
		t := types.NewObject("$ReadableStreamDefaultReader")
		t.AddField("$stream", c.builtinReadableStreamType(), false)
		t.AddField("closed", c.newPromiseType(types.TypeUndefined), false)
		c.result.ReadableStreamDefaultReaderType = t
	}
	return c.result.ReadableStreamDefaultReaderType
}

func (c *Checker) builtinReadableStreamDefaultReaderMember(property string) (types.Type, bool) {
	switch property {
	case "closed":
		return c.newPromiseType(types.TypeUndefined), true
	case "read":
		return types.NewFunction(nil, c.newPromiseType(c.builtinReadableStreamReadResultType())), true
	case "releaseLock":
		return types.NewFunction(nil, types.TypeVoid), true
	case "cancel":
		return types.NewFunction([]types.Param{{Name: "reason", Type: types.TypeAny, Optional: true}}, c.newPromiseType(types.TypeAny)), true
	}
	return nil, false
}

func (c *Checker) builtinReadableStreamDefaultControllerType() *types.ObjectType {
	if c.result.ReadableStreamDefaultControllerType == nil {
		t := types.NewObject("$ReadableStreamDefaultController")
		t.AddField("$stream", c.builtinReadableStreamType(), false)
		t.AddField("desiredSize", types.NewUnion(types.TypeNumber, types.TypeNull), false)
		c.result.ReadableStreamDefaultControllerType = t
	}
	return c.result.ReadableStreamDefaultControllerType
}

func (c *Checker) builtinReadableStreamDefaultControllerMember(property string) (types.Type, bool) {
	switch property {
	case "desiredSize":
		return types.NewUnion(types.TypeNumber, types.TypeNull), true
	case "close":
		return types.NewFunction(nil, types.TypeVoid), true
	case "enqueue":
		return types.NewFunction([]types.Param{{Name: "chunk", Type: types.TypeAny, Optional: true}}, types.TypeVoid), true
	case "error":
		return types.NewFunction([]types.Param{{Name: "e", Type: types.TypeAny, Optional: true}}, types.TypeVoid), true
	}
	return nil, false
}

func (c *Checker) builtinReadableStreamType() *types.ObjectType {
	if c.result.ReadableStreamType == nil {
		t := types.NewObject("$ReadableStream")
		t.AddField("locked", types.TypeBoolean, false)
		t.AddField("$state", types.TypeString, false)
		t.AddField("$queue", types.NewArray(types.TypeAny), false)
		t.AddField("$queueIndex", types.TypeNumber, false)
		t.AddField("$reader", types.TypeAny, false)
		t.AddField("$controller", types.TypeAny, false)
		t.AddField("$source", types.TypeAny, false)
		t.AddField("$pullFn", types.TypeAny, false)
		t.AddField("$cancelFn", types.TypeAny, false)
		t.AddField("$highWaterMark", types.TypeNumber, false)
		t.AddField("$storedError", types.TypeAny, false)
		t.AddField("$disturbFn", types.NewFunction(nil, types.TypeVoid), false)
		c.result.ReadableStreamType = t

		_ = c.builtinReadableStreamDefaultControllerType()
		_ = c.builtinReadableStreamDefaultReaderType()
		_ = c.builtinReadableStreamReadResultType()
	}
	return c.result.ReadableStreamType
}

func (c *Checker) builtinReadableStreamMember(property string) (types.Type, bool) {
	switch property {
	case "locked":
		return types.TypeBoolean, true
	case "cancel":
		return types.NewFunction([]types.Param{{Name: "reason", Type: types.TypeAny, Optional: true}}, c.newPromiseType(types.TypeAny)), true
	case "getReader":
		return types.NewFunction([]types.Param{{Name: "options", Type: types.TypeAny, Optional: true}}, c.builtinReadableStreamDefaultReaderType()), true
	case "pipeThrough":
		return types.NewFunction([]types.Param{
			{Name: "transform", Type: types.TypeAny},
			{Name: "options", Type: types.TypeAny, Optional: true},
		}, c.builtinReadableStreamType()), true
	case "pipeTo":
		return types.NewFunction([]types.Param{
			{Name: "destination", Type: types.TypeAny},
			{Name: "options", Type: types.TypeAny, Optional: true},
		}, c.newPromiseType(types.TypeVoid)), true
	case "tee":
		return types.NewFunction(nil, types.NewArray(c.builtinReadableStreamType())), true
	}
	return nil, false
}

func (c *Checker) builtinReadableStreamStaticMember(property string) (types.Type, bool) {
	switch property {
	case "from":
		return types.NewFunction([]types.Param{{Name: "asyncIterableOrIterable", Type: types.TypeAny}}, c.builtinReadableStreamType()), true
	}
	return nil, false
}

func (c *Checker) builtinWritableStreamDefaultWriterType() *types.ObjectType {
	if c.result.WritableStreamDefaultWriterType == nil {
		t := types.NewObject("$WritableStreamDefaultWriter")
		t.AddField("$stream", c.builtinWritableStreamType(), false)
		t.AddField("closed", c.newPromiseType(types.TypeUndefined), false)
		t.AddField("ready", c.newPromiseType(types.TypeUndefined), false)
		t.AddField("desiredSize", types.NewUnion(types.TypeNumber, types.TypeNull), false)
		c.result.WritableStreamDefaultWriterType = t
	}
	return c.result.WritableStreamDefaultWriterType
}

func (c *Checker) builtinWritableStreamDefaultWriterMember(property string) (types.Type, bool) {
	switch property {
	case "closed", "ready":
		return c.newPromiseType(types.TypeUndefined), true
	case "desiredSize":
		return types.NewUnion(types.TypeNumber, types.TypeNull), true
	case "write":
		return types.NewFunction([]types.Param{{Name: "chunk", Type: types.TypeAny, Optional: true}}, c.newPromiseType(types.TypeVoid)), true
	case "close":
		return types.NewFunction(nil, c.newPromiseType(types.TypeVoid)), true
	case "abort":
		return types.NewFunction([]types.Param{{Name: "reason", Type: types.TypeAny, Optional: true}}, c.newPromiseType(types.TypeAny)), true
	case "releaseLock":
		return types.NewFunction(nil, types.TypeVoid), true
	}
	return nil, false
}

func (c *Checker) builtinWritableStreamDefaultControllerType() *types.ObjectType {
	if c.result.WritableStreamDefaultControllerType == nil {
		t := types.NewObject("$WritableStreamDefaultController")
		t.AddField("$stream", c.builtinWritableStreamType(), false)
		t.AddField("signal", c.builtinAbortSignalType(), false)
		c.result.WritableStreamDefaultControllerType = t
	}
	return c.result.WritableStreamDefaultControllerType
}

func (c *Checker) builtinWritableStreamDefaultControllerMember(property string) (types.Type, bool) {
	switch property {
	case "signal":
		return c.builtinAbortSignalType(), true
	case "error":
		return types.NewFunction([]types.Param{{Name: "e", Type: types.TypeAny, Optional: true}}, types.TypeVoid), true
	}
	return nil, false
}

func (c *Checker) builtinWritableStreamType() *types.ObjectType {
	if c.result.WritableStreamType == nil {
		t := types.NewObject("$WritableStream")
		t.AddField("locked", types.TypeBoolean, false)
		t.AddField("$state", types.TypeString, false)
		t.AddField("$writer", types.TypeAny, false)
		t.AddField("$controller", types.TypeAny, false)
		t.AddField("$sink", types.TypeAny, false)
		t.AddField("$writeFn", types.TypeAny, false)
		t.AddField("$closeFn", types.TypeAny, false)
		t.AddField("$abortFn", types.TypeAny, false)
		t.AddField("$highWaterMark", types.TypeNumber, false)
		t.AddField("$storedError", types.TypeAny, false)
		c.result.WritableStreamType = t

		_ = c.builtinWritableStreamDefaultWriterType()
		_ = c.builtinWritableStreamDefaultControllerType()
	}
	return c.result.WritableStreamType
}

func (c *Checker) builtinWritableStreamMember(property string) (types.Type, bool) {
	switch property {
	case "locked":
		return types.TypeBoolean, true
	case "abort":
		return types.NewFunction([]types.Param{{Name: "reason", Type: types.TypeAny, Optional: true}}, c.newPromiseType(types.TypeAny)), true
	case "close":
		return types.NewFunction(nil, c.newPromiseType(types.TypeVoid)), true
	case "getWriter":
		return types.NewFunction(nil, c.builtinWritableStreamDefaultWriterType()), true
	}
	return nil, false
}

func (c *Checker) builtinTransformStreamDefaultControllerType() *types.ObjectType {
	if c.result.TransformStreamDefaultControllerType == nil {
		t := types.NewObject("$TransformStreamDefaultController")
		t.AddField("$transformStream", c.builtinTransformStreamType(), false)
		t.AddField("desiredSize", types.NewUnion(types.TypeNumber, types.TypeNull), false)
		c.result.TransformStreamDefaultControllerType = t
	}
	return c.result.TransformStreamDefaultControllerType
}

func (c *Checker) builtinTransformStreamDefaultControllerMember(property string) (types.Type, bool) {
	switch property {
	case "desiredSize":
		return types.NewUnion(types.TypeNumber, types.TypeNull), true
	case "enqueue":
		return types.NewFunction([]types.Param{{Name: "chunk", Type: types.TypeAny, Optional: true}}, types.TypeVoid), true
	case "error":
		return types.NewFunction([]types.Param{{Name: "reason", Type: types.TypeAny, Optional: true}}, types.TypeVoid), true
	case "terminate":
		return types.NewFunction(nil, types.TypeVoid), true
	}
	return nil, false
}

func (c *Checker) builtinTransformStreamType() *types.ObjectType {
	if c.result.TransformStreamType == nil {
		t := types.NewObject("$TransformStream")
		t.AddField("readable", c.builtinReadableStreamType(), false)
		t.AddField("writable", c.builtinWritableStreamType(), false)
		t.AddField("$controller", types.TypeAny, false)
		t.AddField("$transformer", types.TypeAny, false)
		t.AddField("$transformFn", types.TypeAny, false)
		t.AddField("$flushFn", types.TypeAny, false)
		c.result.TransformStreamType = t

		_ = c.builtinTransformStreamDefaultControllerType()
	}
	return c.result.TransformStreamType
}

func (c *Checker) builtinTransformStreamMember(property string) (types.Type, bool) {
	switch property {
	case "readable":
		return c.builtinReadableStreamType(), true
	case "writable":
		return c.builtinWritableStreamType(), true
	}
	return nil, false
}

func (c *Checker) builtinByteLengthQueuingStrategyType() *types.ObjectType {
	if c.result.ByteLengthQueuingStrategyType == nil {
		t := types.NewObject("$ByteLengthQueuingStrategy")
		t.AddField("highWaterMark", types.TypeNumber, false)
		c.result.ByteLengthQueuingStrategyType = t
	}
	return c.result.ByteLengthQueuingStrategyType
}

func (c *Checker) builtinByteLengthQueuingStrategyMember(property string) (types.Type, bool) {
	switch property {
	case "highWaterMark":
		return types.TypeNumber, true
	case "size":
		return types.NewFunction([]types.Param{{Name: "chunk", Type: types.TypeAny, Optional: true}}, types.TypeNumber), true
	}
	return nil, false
}

func (c *Checker) builtinCountQueuingStrategyType() *types.ObjectType {
	if c.result.CountQueuingStrategyType == nil {
		t := types.NewObject("$CountQueuingStrategy")
		t.AddField("highWaterMark", types.TypeNumber, false)
		c.result.CountQueuingStrategyType = t
	}
	return c.result.CountQueuingStrategyType
}

func (c *Checker) builtinCountQueuingStrategyMember(property string) (types.Type, bool) {
	switch property {
	case "highWaterMark":
		return types.TypeNumber, true
	case "size":
		return types.NewFunction([]types.Param{{Name: "chunk", Type: types.TypeAny, Optional: true}}, types.TypeNumber), true
	}
	return nil, false
}

func (c *Checker) builtinTextEncoderStreamType() *types.ObjectType {
	if c.result.TextEncoderStreamType == nil {
		t := types.NewObject("$TextEncoderStream")
		t.AddField("readable", c.builtinReadableStreamType(), false)
		t.AddField("writable", c.builtinWritableStreamType(), false)
		t.AddField("encoding", types.TypeString, false)
		t.AddField("$transform", c.builtinTransformStreamType(), false)
		c.result.TextEncoderStreamType = t
	}
	return c.result.TextEncoderStreamType
}

func (c *Checker) builtinTextEncoderStreamMember(property string) (types.Type, bool) {
	switch property {
	case "readable":
		return c.builtinReadableStreamType(), true
	case "writable":
		return c.builtinWritableStreamType(), true
	case "encoding":
		return types.TypeString, true
	}
	return nil, false
}

func (c *Checker) builtinTextDecoderStreamType() *types.ObjectType {
	if c.result.TextDecoderStreamType == nil {
		t := types.NewObject("$TextDecoderStream")
		t.AddField("readable", c.builtinReadableStreamType(), false)
		t.AddField("writable", c.builtinWritableStreamType(), false)
		t.AddField("encoding", types.TypeString, false)
		t.AddField("fatal", types.TypeBoolean, false)
		t.AddField("ignoreBOM", types.TypeBoolean, false)
		t.AddField("$transform", c.builtinTransformStreamType(), false)
		c.result.TextDecoderStreamType = t
	}
	return c.result.TextDecoderStreamType
}

func (c *Checker) builtinTextDecoderStreamMember(property string) (types.Type, bool) {
	switch property {
	case "readable":
		return c.builtinReadableStreamType(), true
	case "writable":
		return c.builtinWritableStreamType(), true
	case "encoding":
		return types.TypeString, true
	case "fatal", "ignoreBOM":
		return types.TypeBoolean, true
	}
	return nil, false
}
