package builtins

import (
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// URLInitializer provides the WHATWG URL and URLSearchParams classes (#624),
// built on net/url with the WHATWG rules that matter most in practice for
// the special schemes (http, https, ws, wss, ftp, file): lower-cased host,
// default port dropped, "/" for an empty path, backslash as slash.
type URLInitializer struct{}

func (u *URLInitializer) Name() string  { return "URL" }
func (u *URLInitializer) Priority() int { return 430 }

var specialSchemePorts = map[string]string{
	"http": "80", "https": "443", "ws": "80", "wss": "443", "ftp": "21", "file": "",
}

func (u *URLInitializer) InitTypes(ctx *TypeContext) error {
	optStr := func(params ...types.Type) *types.ObjectType {
		opt := make([]bool, len(params))
		for i := range opt {
			opt[i] = i > 0
		}
		return types.NewFunctionType(&types.Signature{ParameterTypes: params, ReturnType: types.Void, OptionalParams: opt})
	}
	params := types.NewObjectType().
		WithProperty("size", types.Number).
		WithProperty("append", types.NewSimpleFunction([]types.Type{types.String, types.String}, types.Void)).
		WithProperty("delete", optStr(types.String, types.String)).
		WithProperty("get", types.NewSimpleFunction([]types.Type{types.String}, types.NewUnionType(types.String, types.Null))).
		WithProperty("getAll", types.NewSimpleFunction([]types.Type{types.String}, &types.ArrayType{ElementType: types.String})).
		WithProperty("has", types.NewFunctionType(&types.Signature{ParameterTypes: []types.Type{types.String, types.String}, ReturnType: types.Boolean, OptionalParams: []bool{false, true}})).
		WithProperty("set", types.NewSimpleFunction([]types.Type{types.String, types.String}, types.Void)).
		WithProperty("sort", types.NewSimpleFunction(nil, types.Void)).
		WithProperty("toString", types.NewSimpleFunction(nil, types.String)).
		WithProperty("forEach", types.NewFunctionType(&types.Signature{ParameterTypes: []types.Type{types.Any, types.Any}, ReturnType: types.Void, OptionalParams: []bool{false, true}})).
		WithProperty("entries", types.NewSimpleFunction(nil, types.Any)).
		WithProperty("keys", types.NewSimpleFunction(nil, types.Any)).
		WithProperty("values", types.NewSimpleFunction(nil, types.Any))
	paramsCtor := types.NewObjectType().
		WithSimpleConstructSignature([]types.Type{}, params).
		WithSimpleConstructSignature([]types.Type{types.Any}, params).
		WithProperty("prototype", params)
	if err := ctx.DefineGlobal("URLSearchParams", paramsCtor); err != nil {
		return err
	}

	inst := types.NewObjectType()
	for _, p := range []string{"href", "origin", "protocol", "username", "password", "host", "hostname", "port", "pathname", "search", "hash"} {
		inst = inst.WithProperty(p, types.String)
	}
	inst = inst.WithProperty("searchParams", params).
		WithProperty("toString", types.NewSimpleFunction(nil, types.String)).
		WithProperty("toJSON", types.NewSimpleFunction(nil, types.String))
	strOrURL := types.NewUnionType(types.String, inst)
	ctor := types.NewObjectType().
		WithSimpleConstructSignature([]types.Type{strOrURL}, inst).
		WithSimpleConstructSignature([]types.Type{strOrURL, strOrURL}, inst).
		WithProperty("prototype", inst).
		WithProperty("canParse", types.NewFunctionType(&types.Signature{ParameterTypes: []types.Type{strOrURL, strOrURL}, ReturnType: types.Boolean, OptionalParams: []bool{false, true}})).
		WithProperty("parse", types.NewFunctionType(&types.Signature{ParameterTypes: []types.Type{strOrURL, strOrURL}, ReturnType: types.NewUnionType(inst, types.Null), OptionalParams: []bool{false, true}}))
	return ctx.DefineGlobal("URL", ctor)
}

// parseWebURL parses input (against base when given) into a normalized
// absolute URL.
func parseWebURL(input string, base *string) (*url.URL, bool) {
	clean := func(s string) string {
		return strings.TrimFunc(s, func(r rune) bool { return r <= ' ' })
	}
	input = clean(input)
	var u *url.URL
	if base != nil {
		b, ok := parseWebURL(*base, nil)
		if !ok {
			return nil, false
		}
		if isSpecialScheme(b.Scheme) {
			input = strings.ReplaceAll(input, "\\", "/")
		}
		ref, err := url.Parse(input)
		if err != nil {
			return nil, false
		}
		if ref.Scheme != "" {
			return parseWebURL(input, nil)
		}
		u = b.ResolveReference(ref)
	} else {
		if i := strings.Index(input, ":"); i > 0 && isSpecialScheme(strings.ToLower(input[:i])) {
			input = strings.ReplaceAll(input, "\\", "/")
		}
		parsed, err := url.Parse(input)
		if err != nil || parsed.Scheme == "" {
			return nil, false
		}
		u = parsed
	}
	return normalizeWebURL(u)
}

// removeDotSegments resolves "." and ".." path segments (RFC 3986 5.2.4),
// keeping a trailing slash.
func removeDotSegments(p string) string {
	segs := strings.Split(p, "/")
	out := make([]string, 0, len(segs))
	for i, seg := range segs {
		last := i == len(segs)-1
		switch seg {
		case ".", "%2e", "%2E":
			if last {
				out = append(out, "")
			}
		case "..", ".%2e", "%2e.", "%2e%2e", ".%2E", "%2E.", "%2E%2E":
			if len(out) > 1 {
				out = out[:len(out)-1]
			}
			if last {
				out = append(out, "")
			}
		default:
			out = append(out, seg)
		}
	}
	res := strings.Join(out, "/")
	if !strings.HasPrefix(res, "/") {
		res = "/" + res
	}
	return res
}

// encodeQuerySet percent-encodes the bytes of a raw query that the WHATWG
// query percent-encode set covers (controls, space, ", #, <, >, and ' for
// special schemes); existing escapes are kept.
func encodeQuerySet(q string, special bool) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(q); i++ {
		c := q[i]
		if c <= ' ' || c == '"' || c == '#' || c == '<' || c == '>' || c == 0x7F || (special && c == '\'') {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func isSpecialScheme(s string) bool {
	_, ok := specialSchemePorts[s]
	return ok
}

func normalizeWebURL(u *url.URL) (*url.URL, bool) {
	u.Scheme = strings.ToLower(u.Scheme)
	u.RawQuery = encodeQuerySet(u.RawQuery, isSpecialScheme(u.Scheme))
	if u.Opaque == "" && strings.HasPrefix(u.Path, "/") {
		u.Path = removeDotSegments(u.Path)
		u.RawPath = ""
	}
	if !isSpecialScheme(u.Scheme) {
		return u, true
	}
	if u.Opaque != "" {
		// "http:example.com" - a special URL always has an authority.
		rest := strings.TrimLeft(u.Opaque, "/")
		reparsed, err := url.Parse(u.Scheme + "://" + rest)
		if err != nil {
			return nil, false
		}
		u = reparsed
	}
	u.Host = strings.ToLower(u.Host)
	if u.Host == "" && u.Scheme != "file" {
		return nil, false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n > 65535 {
			return nil, false
		}
		if strconv.Itoa(n) == specialSchemePorts[u.Scheme] {
			u.Host = strings.TrimSuffix(u.Host, ":"+port)
		} else if strconv.Itoa(n) != port {
			u.Host = strings.TrimSuffix(u.Host, ":"+port) + ":" + strconv.Itoa(n)
		}
	}
	if u.Path == "" {
		u.Path = "/"
		u.RawPath = ""
	}
	return u, true
}

func (u *URLInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	paramsProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	urlProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	paramsCtor := makeConstructor(vmInstance, "URLSearchParams", paramsProto, func(args []vm.Value) (vm.Value, error) {
		sp := &searchParams{}
		if len(args) > 0 {
			if err := sp.init(vmInstance, args[0]); err != nil {
				return vm.Undefined, err
			}
		}
		return newSearchParamsObject(vmInstance, sp, paramsProto), nil
	})
	if err := ctx.DefineGlobal("URLSearchParams", paramsCtor); err != nil {
		return err
	}

	toStringArg := func(v vm.Value) string {
		if v.IsObject() {
			if href, ok := v.AsPlainObject().GetInternal("[[URLHref]]"); ok {
				return href.ToString()
			}
		}
		return v.ToString()
	}
	parseArgs := func(args []vm.Value) (*url.URL, bool) {
		input := ""
		if len(args) > 0 {
			input = toStringArg(args[0])
		}
		var base *string
		if len(args) > 1 && args[1].Type() != vm.TypeUndefined {
			b := toStringArg(args[1])
			base = &b
		}
		return parseWebURL(input, base)
	}

	urlCtor := makeConstructor(vmInstance, "URL", urlProto, func(args []vm.Value) (vm.Value, error) {
		u, ok := parseArgs(args)
		if !ok {
			input := ""
			if len(args) > 0 {
				input = toStringArg(args[0])
			}
			return vm.Undefined, vmInstance.NewTypeError("Invalid URL: " + input)
		}
		return newURLObject(vmInstance, u, urlProto, paramsProto), nil
	})
	props := urlCtor.AsNativeFunctionWithProps().Properties
	props.SetOwnNonEnumerable("canParse", vm.NewNativeFunction(1, false, "canParse", func(args []vm.Value) (vm.Value, error) {
		_, ok := parseArgs(args)
		return vm.BooleanValue(ok), nil
	}))
	props.SetOwnNonEnumerable("parse", vm.NewNativeFunction(1, false, "parse", func(args []vm.Value) (vm.Value, error) {
		u, ok := parseArgs(args)
		if !ok {
			return vm.Null, nil
		}
		return newURLObject(vmInstance, u, urlProto, paramsProto), nil
	}))
	return ctx.DefineGlobal("URL", urlCtor)
}

// makeConstructor wraps fn as a constructor whose prototype is proto.
func makeConstructor(vmInstance *vm.VM, name string, proto *vm.PlainObject, fn func(args []vm.Value) (vm.Value, error)) vm.Value {
	ctor := vm.NewConstructorWithProps(0, true, name, fn)
	ctor.AsNativeFunctionWithProps().Properties.DefineFixedProperty("prototype", vm.NewValueFromPlainObject(proto))
	proto.SetOwnNonEnumerable("constructor", ctor)
	return ctor
}

// webURLHref serializes u the way URL.href does.
func webURLHref(u *url.URL) string {
	return u.String()
}

func newURLObject(vmInstance *vm.VM, u *url.URL, proto, paramsProto *vm.PlainObject) vm.Value {
	objVal := vm.NewObject(vm.NewValueFromPlainObject(proto))
	obj := objVal.AsPlainObject()
	sp := &searchParams{}
	sp.parse(u.RawQuery)
	sync := func() {
		obj.SetInternal("[[URLHref]]", vm.NewString(webURLHref(u)))
	}
	sp.onChange = func() {
		u.RawQuery = sp.serialize()
		u.ForceQuery = false
		sync()
	}
	sync()
	paramsVal := newSearchParamsObject(vmInstance, sp, paramsProto)

	f, t := false, true
	accessor := func(name string, get func() string, set func(string) error) {
		getter := vm.NewNativeFunction(0, false, "get "+name, func(args []vm.Value) (vm.Value, error) {
			return vm.NewString(get()), nil
		})
		setter := vm.Undefined
		if set != nil {
			setter = vm.NewNativeFunction(1, false, "set "+name, func(args []vm.Value) (vm.Value, error) {
				v := ""
				if len(args) > 0 {
					v = args[0].ToString()
				}
				if err := set(v); err != nil {
					return vm.Undefined, err
				}
				sync()
				return vm.Undefined, nil
			})
		}
		obj.DefineAccessorProperty(name, getter, true, setter, set != nil, &f, &t)
	}
	// reparse applies a component change by re-parsing the serialized URL,
	// keeping the old URL if the result is invalid (setters never throw,
	// except href).
	reparse := func(mutate func(c *url.URL)) {
		c := *u
		mutate(&c)
		if nu, ok := parseWebURL(c.String(), nil); ok {
			*u = *nu
			sp.parse(u.RawQuery)
		}
	}

	accessor("href", func() string { return webURLHref(u) }, func(v string) error {
		nu, ok := parseWebURL(v, nil)
		if !ok {
			return vmInstance.NewTypeError("Invalid URL: " + v)
		}
		*u = *nu
		sp.parse(u.RawQuery)
		return nil
	})
	accessor("origin", func() string {
		if isSpecialScheme(u.Scheme) && u.Scheme != "file" {
			return u.Scheme + "://" + u.Host
		}
		return "null"
	}, nil)
	accessor("protocol", func() string { return u.Scheme + ":" }, func(v string) error {
		scheme := strings.ToLower(strings.TrimSuffix(strings.SplitN(v, ":", 2)[0], ":"))
		if scheme != "" && isSpecialScheme(scheme) == isSpecialScheme(u.Scheme) {
			reparse(func(c *url.URL) { c.Scheme = scheme })
		}
		return nil
	})
	accessor("username", func() string {
		if u.User == nil {
			return ""
		}
		return u.User.Username()
	}, func(v string) error {
		reparse(func(c *url.URL) {
			pw, has := "", false
			if c.User != nil {
				pw, has = c.User.Password()
			}
			if has {
				c.User = url.UserPassword(v, pw)
			} else if v != "" {
				c.User = url.User(v)
			} else {
				c.User = nil
			}
		})
		return nil
	})
	accessor("password", func() string {
		if u.User == nil {
			return ""
		}
		pw, _ := u.User.Password()
		return pw
	}, func(v string) error {
		reparse(func(c *url.URL) {
			name := ""
			if c.User != nil {
				name = c.User.Username()
			}
			if v == "" && name == "" {
				c.User = nil
			} else if v == "" {
				c.User = url.User(name)
			} else {
				c.User = url.UserPassword(name, v)
			}
		})
		return nil
	})
	accessor("host", func() string { return u.Host }, func(v string) error {
		reparse(func(c *url.URL) { c.Host = v })
		return nil
	})
	accessor("hostname", func() string {
		h := u.Host
		if port := u.Port(); port != "" {
			h = strings.TrimSuffix(h, ":"+port)
		}
		return h
	}, func(v string) error {
		reparse(func(c *url.URL) {
			if port := c.Port(); port != "" {
				c.Host = v + ":" + port
			} else {
				c.Host = v
			}
		})
		return nil
	})
	accessor("port", func() string { return u.Port() }, func(v string) error {
		digits := v
		for i, r := range v {
			if r < '0' || r > '9' {
				digits = v[:i]
				break
			}
		}
		reparse(func(c *url.URL) {
			host := c.Host
			if port := c.Port(); port != "" {
				host = strings.TrimSuffix(host, ":"+port)
			}
			if digits != "" {
				host += ":" + digits
			}
			c.Host = host
		})
		return nil
	})
	accessor("pathname", func() string {
		if u.Opaque != "" {
			return u.Opaque
		}
		return u.EscapedPath()
	}, func(v string) error {
		if u.Opaque != "" {
			return nil
		}
		reparse(func(c *url.URL) {
			if !strings.HasPrefix(v, "/") {
				v = "/" + v
			}
			c.Path, c.RawPath = v, ""
		})
		return nil
	})
	accessor("search", func() string {
		if u.RawQuery == "" {
			return ""
		}
		return "?" + u.RawQuery
	}, func(v string) error {
		u.RawQuery = strings.TrimPrefix(v, "?")
		u.ForceQuery = false
		sp.parse(u.RawQuery)
		return nil
	})
	accessor("hash", func() string {
		if u.Fragment == "" {
			return ""
		}
		return "#" + u.EscapedFragment()
	}, func(v string) error {
		v = strings.TrimPrefix(v, "#")
		u.Fragment, u.RawFragment = v, ""
		if f, err := url.PathUnescape(v); err == nil {
			u.Fragment = f
		}
		return nil
	})
	obj.DefineAccessorProperty("searchParams", vm.NewNativeFunction(0, false, "get searchParams", func(args []vm.Value) (vm.Value, error) {
		return paramsVal, nil
	}), true, vm.Undefined, false, &f, &t)
	hrefFn := func(args []vm.Value) (vm.Value, error) { return vm.NewString(webURLHref(u)), nil }
	obj.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", hrefFn))
	obj.SetOwnNonEnumerable("toJSON", vm.NewNativeFunction(0, false, "toJSON", hrefFn))
	return objVal
}

// searchParams is a URLSearchParams list; onChange (set for a URL's
// searchParams) runs after every mutation to update the URL's query.
type searchParams struct {
	pairs    [][2]string
	onChange func()
}

func (sp *searchParams) changed() {
	if sp.onChange != nil {
		sp.onChange()
	}
}

// parse replaces the list with an application/x-www-form-urlencoded string.
func (sp *searchParams) parse(query string) {
	sp.pairs = sp.pairs[:0]
	query = strings.TrimPrefix(query, "?")
	for _, part := range strings.Split(query, "&") {
		if part == "" {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		sp.pairs = append(sp.pairs, [2]string{formDecode(name), formDecode(value)})
	}
}

func formDecode(s string) string {
	s = strings.ReplaceAll(s, "+", " ")
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			n, _ := strconv.ParseUint(s[i+1:i+3], 16, 8)
			b.WriteByte(byte(n))
			i += 2
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// formEncode is the application/x-www-form-urlencoded byte serializer.
func formEncode(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for _, c := range []byte(vm.UTF16ToString(vm.StringToUTF16(s))) {
		switch {
		case c == ' ':
			b.WriteByte('+')
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '*', c == '-', c == '.', c == '_':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}

func (sp *searchParams) serialize() string {
	parts := make([]string, len(sp.pairs))
	for i, p := range sp.pairs {
		parts[i] = formEncode(p[0]) + "=" + formEncode(p[1])
	}
	return strings.Join(parts, "&")
}

// init fills a new list from the constructor argument: a string, another
// URLSearchParams, a sequence of pairs, or a record.
func (sp *searchParams) init(vmInstance *vm.VM, init vm.Value) error {
	switch {
	case init.Type() == vm.TypeUndefined || init.Type() == vm.TypeNull:
		return nil
	case init.IsString():
		sp.parse(init.ToString())
		return nil
	case init.Type() == vm.TypeArray:
		arr := init.AsArray()
		for i := 0; i < arr.Length(); i++ {
			pair := arr.Get(i)
			if pair.Type() != vm.TypeArray || pair.AsArray().Length() != 2 {
				return vmInstance.NewTypeError("Failed to construct 'URLSearchParams': Each query pair must be an iterable [name, value] tuple")
			}
			sp.pairs = append(sp.pairs, [2]string{pair.AsArray().Get(0).ToString(), pair.AsArray().Get(1).ToString()})
		}
		return nil
	case init.Type() == vm.TypeMap:
		init.AsMap().ForEach(func(k, v vm.Value) {
			sp.pairs = append(sp.pairs, [2]string{k.ToString(), v.ToString()})
		})
		return nil
	case init.IsObject():
		if src, ok := searchParamsOf(init); ok {
			sp.pairs = append(sp.pairs, src.pairs...)
			return nil
		}
		keys, err := objectKeysWithVM(vmInstance, []vm.Value{init})
		if err != nil {
			return err
		}
		ka := keys.AsArray()
		for i := 0; i < ka.Length(); i++ {
			name := ka.Get(i).ToString()
			v, err := vmInstance.GetProperty(init, name)
			if err != nil {
				return err
			}
			sp.pairs = append(sp.pairs, [2]string{name, v.ToString()})
		}
		return nil
	}
	sp.parse(init.ToString())
	return nil
}

// searchParamsKey is the internal slot marking a URLSearchParams object; it
// holds a native function serializing its list, which is how
// `new URLSearchParams(other)` copies another instance's entries.
const searchParamsKey = "[[URLSearchParams]]"

func searchParamsOf(v vm.Value) (*searchParams, bool) {
	if !v.IsObject() {
		return nil, false
	}
	marker, ok := v.AsPlainObject().GetInternal(searchParamsKey)
	if !ok {
		return nil, false
	}
	getPairs := marker.AsNativeFunction()
	if getPairs == nil {
		return nil, false
	}
	res, _ := getPairs.Fn(nil)
	sp := &searchParams{}
	sp.parse(res.ToString())
	return sp, true
}

func newSearchParamsObject(vmInstance *vm.VM, sp *searchParams, proto *vm.PlainObject) vm.Value {
	objVal := vm.NewObject(vm.NewValueFromPlainObject(proto))
	obj := objVal.AsPlainObject()
	obj.SetInternal(searchParamsKey, vm.NewNativeFunction(0, false, "", func([]vm.Value) (vm.Value, error) {
		return vm.NewString(sp.serialize()), nil
	}))
	arg := func(args []vm.Value, i int) (string, bool) {
		if i < len(args) && args[i].Type() != vm.TypeUndefined {
			return args[i].ToString(), true
		}
		return "", false
	}
	method := func(name string, arity int, fn func(args []vm.Value) (vm.Value, error)) {
		obj.SetOwnNonEnumerable(name, vm.NewNativeFunction(arity, false, name, fn))
	}
	method("append", 2, func(args []vm.Value) (vm.Value, error) {
		n, _ := arg(args, 0)
		v, _ := arg(args, 1)
		sp.pairs = append(sp.pairs, [2]string{n, v})
		sp.changed()
		return vm.Undefined, nil
	})
	method("delete", 1, func(args []vm.Value) (vm.Value, error) {
		n, _ := arg(args, 0)
		v, hasV := arg(args, 1)
		kept := sp.pairs[:0]
		for _, p := range sp.pairs {
			if p[0] == n && (!hasV || p[1] == v) {
				continue
			}
			kept = append(kept, p)
		}
		sp.pairs = kept
		sp.changed()
		return vm.Undefined, nil
	})
	method("get", 1, func(args []vm.Value) (vm.Value, error) {
		n, _ := arg(args, 0)
		for _, p := range sp.pairs {
			if p[0] == n {
				return vm.NewString(p[1]), nil
			}
		}
		return vm.Null, nil
	})
	method("getAll", 1, func(args []vm.Value) (vm.Value, error) {
		n, _ := arg(args, 0)
		var out []vm.Value
		for _, p := range sp.pairs {
			if p[0] == n {
				out = append(out, vm.NewString(p[1]))
			}
		}
		return vm.NewArrayWithArgs(out), nil
	})
	method("has", 1, func(args []vm.Value) (vm.Value, error) {
		n, _ := arg(args, 0)
		v, hasV := arg(args, 1)
		for _, p := range sp.pairs {
			if p[0] == n && (!hasV || p[1] == v) {
				return vm.True, nil
			}
		}
		return vm.False, nil
	})
	method("set", 2, func(args []vm.Value) (vm.Value, error) {
		n, _ := arg(args, 0)
		v, _ := arg(args, 1)
		out := sp.pairs[:0]
		found := false
		for _, p := range sp.pairs {
			if p[0] == n {
				if found {
					continue
				}
				found = true
				p[1] = v
			}
			out = append(out, p)
		}
		sp.pairs = out
		if !found {
			sp.pairs = append(sp.pairs, [2]string{n, v})
		}
		sp.changed()
		return vm.Undefined, nil
	})
	method("sort", 0, func(args []vm.Value) (vm.Value, error) {
		// Stable, by UTF-16 code units of the name.
		sort.SliceStable(sp.pairs, func(i, j int) bool {
			a, b := vm.StringToUTF16(sp.pairs[i][0]), vm.StringToUTF16(sp.pairs[j][0])
			for k := 0; k < len(a) && k < len(b); k++ {
				if a[k] != b[k] {
					return a[k] < b[k]
				}
			}
			return len(a) < len(b)
		})
		sp.changed()
		return vm.Undefined, nil
	})
	method("toString", 0, func(args []vm.Value) (vm.Value, error) {
		return vm.NewString(sp.serialize()), nil
	})
	method("forEach", 1, func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 || !args[0].IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("URLSearchParams.forEach: callback is not a function")
		}
		thisArg := vm.Undefined
		if len(args) > 1 {
			thisArg = args[1]
		}
		for i := 0; i < len(sp.pairs); i++ {
			p := sp.pairs[i]
			if _, err := vmInstance.Call(args[0], thisArg, []vm.Value{vm.NewString(p[1]), vm.NewString(p[0]), objVal}); err != nil {
				return vm.Undefined, err
			}
		}
		return vm.Undefined, nil
	})
	// Iterators over a snapshot of the list.
	iter := func(pick func(p [2]string) vm.Value) (vm.Value, error) {
		vals := make([]vm.Value, len(sp.pairs))
		for i, p := range sp.pairs {
			vals[i] = pick(p)
		}
		arr := vm.NewArrayWithArgs(vals)
		values, err := vmInstance.GetProperty(arr, "values")
		if err != nil {
			return vm.Undefined, err
		}
		return vmInstance.Call(values, arr, nil)
	}
	entries := vm.NewNativeFunction(0, false, "entries", func(args []vm.Value) (vm.Value, error) {
		return iter(func(p [2]string) vm.Value {
			return vm.NewArrayWithArgs([]vm.Value{vm.NewString(p[0]), vm.NewString(p[1])})
		})
	})
	obj.SetOwnNonEnumerable("entries", entries)
	obj.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), entries, nil, nil, nil)
	method("keys", 0, func(args []vm.Value) (vm.Value, error) {
		return iter(func(p [2]string) vm.Value { return vm.NewString(p[0]) })
	})
	method("values", 0, func(args []vm.Value) (vm.Value, error) {
		return iter(func(p [2]string) vm.Value { return vm.NewString(p[1]) })
	})
	f, t := false, true
	obj.DefineAccessorProperty("size", vm.NewNativeFunction(0, false, "get size", func(args []vm.Value) (vm.Value, error) {
		return vm.NumberValue(float64(len(sp.pairs))), nil
	}), true, vm.Undefined, false, &f, &t)
	return objVal
}
