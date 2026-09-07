package sema

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (c *Checker) checkNewExpr(e *ast.NewExpr) types.Type {
	if e.ClassName == "Blob" {
		t := c.builtinBlobType()
		if len(e.Args) > 2 {
			c.error(e.Span(), "TS2554", "Blob constructor expects at most 2 arguments.")
		}
		for _, arg := range e.Args {
			c.checkExpr(arg)
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "File" {
		t := c.builtinFileType()
		if len(e.Args) < 2 || len(e.Args) > 3 {
			c.error(e.Span(), "TS2554", "File constructor expects 2 or 3 arguments.")
		} else {
			c.checkExpr(e.Args[0])
			nameType := c.checkExpr(e.Args[1])
			if nameType != types.TypeString && nameType != types.TypeAny {
				c.error(e.Args[1].Span(), "TS2345", "File name must be a string.")
			}
			if len(e.Args) == 3 {
				c.checkExpr(e.Args[2])
			}
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "FormData" {
		t := c.builtinFormDataType()
		if len(e.Args) > 1 {
			c.error(e.Span(), "TS2554", "FormData constructor expects at most 1 argument.")
		}
		for _, arg := range e.Args {
			c.checkExpr(arg)
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "Request" {
		t := c.builtinRequestType()
		if len(e.Args) < 1 || len(e.Args) > 2 {
			c.error(e.Span(), "TS2554", "Request expects input and optional init.")
		} else {
			it := c.checkExpr(e.Args[0])
			if it != types.TypeString && it != t && it != types.TypeAny {
				c.error(e.Args[0].Span(), "TS2345", "Request input must be a string or Request.")
			}
			if len(e.Args) == 2 {
				c.checkExpr(e.Args[1])
			}
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "Headers" {
		t := c.builtinHeadersType()
		if len(e.Args) > 1 {
			c.error(e.Span(), "TS2554", "Headers constructor expects at most 1 argument.")
		}
		for _, arg := range e.Args {
			c.checkExpr(arg)
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "ReadableStream" {
		t := c.builtinReadableStreamType()
		for _, arg := range e.Args {
			c.checkExpr(arg)
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "ReadableStreamDefaultReader" {
		t := c.builtinReadableStreamDefaultReaderType()
		for _, arg := range e.Args {
			c.checkExpr(arg)
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "WritableStream" {
		t := c.builtinWritableStreamType()
		for _, arg := range e.Args {
			c.checkExpr(arg)
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "WritableStreamDefaultWriter" {
		t := c.builtinWritableStreamDefaultWriterType()
		for _, arg := range e.Args {
			c.checkExpr(arg)
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "TransformStream" {
		t := c.builtinTransformStreamType()
		for _, arg := range e.Args {
			c.checkExpr(arg)
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "ByteLengthQueuingStrategy" {
		t := c.builtinByteLengthQueuingStrategyType()
		for _, arg := range e.Args {
			c.checkExpr(arg)
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "CountQueuingStrategy" {
		t := c.builtinCountQueuingStrategyType()
		for _, arg := range e.Args {
			c.checkExpr(arg)
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "TextEncoderStream" {
		t := c.builtinTextEncoderStreamType()
		for _, arg := range e.Args {
			c.checkExpr(arg)
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "TextDecoderStream" {
		t := c.builtinTextDecoderStreamType()
		for _, arg := range e.Args {
			c.checkExpr(arg)
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "URLPattern" {
		t := c.builtinURLPatternType()
		if len(e.Args) < 1 || len(e.Args) > 2 {
			c.error(e.Span(), "TS2554", "URLPattern expects an input and optional baseURL.")
		} else {
			c.checkExpr(e.Args[0])
			if len(e.Args) == 2 {
				baseType := c.checkExpr(e.Args[1])
				if baseType != types.TypeString && baseType != types.TypeAny {
					c.error(e.Args[1].Span(), "TS2345", "URLPattern baseURL must be a string.")
				}
			}
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "URL" {
		t := c.builtinURLType()
		if len(e.Args) < 1 || len(e.Args) > 2 {
			c.error(e.Span(), "TS2554", "URL expects an input and optional base URL.")
		} else {
			if c.checkExpr(e.Args[0]) != types.TypeString {
				c.error(e.Args[0].Span(), "TS2345", "URL input must be a string.")
			}
			if len(e.Args) == 2 {
				baseType := c.checkExpr(e.Args[1])
				if baseType != types.TypeString && baseType != t {
					c.error(e.Args[1].Span(), "TS2345", "URL base must be a string or URL.")
				}
			}
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "ArrayBuffer" {
		if len(e.Args) != 1 {
			c.error(e.Span(), "TS2554", "ArrayBuffer expects exactly one byteLength argument.")
		} else if c.checkExpr(e.Args[0]) != types.TypeNumber {
			c.error(e.Args[0].Span(), "TS2345", "ArrayBuffer byteLength must be a number.")
		}
		t := c.builtinArrayBufferType()
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "Uint8Array" {
		if len(e.Args) != 1 {
			c.error(e.Span(), "TS2554", "Uint8Array currently expects one length or ArrayBuffer argument.")
		} else {
			at := c.checkExpr(e.Args[0])
			if at != types.TypeNumber && at != c.builtinArrayBufferType() {
				c.error(e.Args[0].Span(), "TS2345", "Uint8Array argument must be a number or ArrayBuffer.")
			}
		}
		t := c.builtinUint8ArrayType()
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "URLSearchParams" {
		if len(e.Args) > 1 {
			c.error(e.Span(), "TS2554", "URLSearchParams expects at most one string argument.")
		} else if len(e.Args) == 1 && c.checkExpr(e.Args[0]) != types.TypeString {
			c.error(e.Args[0].Span(), "TS2345", "URLSearchParams init must be a string.")
		}
		t := c.builtinURLSearchParamsType()
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "TextEncoder" {
		if len(e.Args) != 0 {
			c.error(e.Span(), "TS2554", "TextEncoder expects no arguments.")
		}
		t := c.builtinTextEncoderType()
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "TextDecoder" {
		if len(e.Args) > 2 {
			c.error(e.Span(), "TS2554", "TextDecoder expects optional label and options arguments.")
		}
		if len(e.Args) > 0 && c.checkExpr(e.Args[0]) != types.TypeString {
			c.error(e.Args[0].Span(), "TS2345", "TextDecoder label must be a string.")
		}
		if len(e.Args) > 1 {
			c.checkExpr(e.Args[1])
		}
		t := c.builtinTextDecoderType()
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "AbortController" {
		if len(e.Args) != 0 {
			c.error(e.Span(), "TS2554", "AbortController expects no arguments.")
		}
		t := c.builtinAbortControllerType()
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "EventTarget" {
		if len(e.Args) != 0 {
			c.error(e.Span(), "TS2554", "EventTarget expects no arguments.")
		}
		t := c.builtinEventTargetType()
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "CustomEvent" || e.ClassName == "MessageEvent" || e.ClassName == "ErrorEvent" {
		if len(e.Args) < 1 || len(e.Args) > 2 {
			c.error(e.Span(), "TS2554", e.ClassName+" expects a type and optional init dictionary.")
		} else {
			if c.checkExpr(e.Args[0]) != types.TypeString {
				c.error(e.Args[0].Span(), "TS2345", e.ClassName+" type must be a string.")
			}
			if len(e.Args) == 2 {
				c.checkExpr(e.Args[1])
			}
		}
		var t *types.ObjectType
		switch e.ClassName {
		case "CustomEvent":
			t = c.builtinCustomEventType()
		case "MessageEvent":
			t = c.builtinMessageEventType()
		default:
			t = c.builtinErrorEventType()
		}
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "Event" {
		if len(e.Args) < 1 || len(e.Args) > 2 {
			c.error(e.Span(), "TS2554", "Event expects a type and optional EventInit.")
		} else {
			if c.checkExpr(e.Args[0]) != types.TypeString {
				c.error(e.Args[0].Span(), "TS2345", "Event type must be a string.")
			}
			if len(e.Args) == 2 {
				c.checkExpr(e.Args[1])
			}
		}
		t := c.builtinEventType()
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "DOMException" {
		if len(e.Args) > 2 {
			c.error(e.Span(), "TS2554", "DOMException expects optional message and name arguments.")
		}
		for _, arg := range e.Args {
			if c.checkExpr(arg) != types.TypeString {
				c.error(arg.Span(), "TS2345", "DOMException message and name must be strings.")
			}
		}
		t := c.builtinDOMExceptionType()
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "RegExp" {
		if len(e.Args) < 1 || len(e.Args) > 2 {
			c.error(e.Span(), "TS2554", "RegExp expects a pattern and optional flags.")
		}
		for _, arg := range e.Args {
			if c.checkExpr(arg) != types.TypeString {
				c.error(arg.Span(), "TS2345", "Native RegExp pattern and flags must be strings.")
			}
		}
		t := c.builtinRegExpType()
		c.result.Types[e] = t
		return t
	}
	if e.ClassName == "Date" {
		if len(e.Args) != 1 {
			c.error(e.Span(), "TS2554", "Native Date constructor currently expects exactly one number or ISO string argument.")
		} else {
			at := c.checkExpr(e.Args[0])
			if at != types.TypeNumber && at != types.TypeString {
				c.error(e.Args[0].Span(), "TS2345", fmt.Sprintf("Date constructor argument must be number or string, got '%s'.", at))
			}
		}
		date := c.builtinDateType()
		c.result.Types[e] = date
		return date
	}
	if e.ClassName == "Map" || e.ClassName == "Set" {
		want := 1
		if e.ClassName == "Map" {
			want = 2
		}
		if len(e.TypeArgs) != want {
			c.error(e.Span(), "TS2558", fmt.Sprintf("%s expects %d type arguments, got %d.", e.ClassName, want, len(e.TypeArgs)))
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}
		args := make([]types.Type, want)
		for i, node := range e.TypeArgs {
			args[i] = c.resolveTypeNode(node)
		}
		key, value := args[0], types.TypeUndefined
		if e.ClassName == "Map" {
			value = args[1]
		}
		info := c.builtinCollection(e.ClassName, key, value)
		c.result.Types[e] = info.Instance
		return info.Instance
	}
	info := c.result.Classes[e.ClassName]
	if info == nil {
		c.error(e.Span(), "TS2304", fmt.Sprintf("Cannot find class '%s'.", e.ClassName))
		c.result.Types[e] = types.TypeAny
		return types.TypeAny
	}
	if len(info.TypeParams) > 0 {
		if len(e.TypeArgs) != len(info.TypeParams) {
			c.error(e.Span(), "TS2558", fmt.Sprintf("Generic class '%s' expects %d type arguments, got %d.", e.ClassName, len(info.TypeParams), len(e.TypeArgs)))
		} else {
			typeArgs := make([]types.Type, len(e.TypeArgs))
			for i, node := range e.TypeArgs {
				typeArgs[i] = c.resolveTypeNode(node)
			}
			spec := c.specializeClass(info, typeArgs)
			info = spec
			c.result.GenericClasses[e] = spec
		}
	} else if len(e.TypeArgs) > 0 {
		c.error(e.Span(), "TS2558", fmt.Sprintf("Class '%s' is not generic.", e.ClassName))
	}
	ctor := info.Constructor
	for i, arg := range e.Args {
		at := c.checkExpr(arg)
		if ctor != nil && i < len(ctor.Params) && !at.AssignableTo(ctor.Params[i].Type) {
			c.error(arg.Span(), "TS2345", fmt.Sprintf("Argument of type '%s' is not assignable to constructor parameter '%s'.", at, ctor.Params[i].Type))
		}
	}
	c.result.Types[e] = info.Instance
	return info.Instance
}
