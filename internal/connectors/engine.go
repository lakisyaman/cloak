package connectors

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/lakisyaman/cloak/internal/contextstore"
	"github.com/lakisyaman/cloak/internal/secrets"
)

type Invocation struct{ Args, Env []string }
type Inputs struct {
	Passthrough bool
	Overrides   map[string]bool
}

// Detect parses enough native syntax to identify declared inputs. It never
// rewrites caller arguments, and unknown flags do not disable Activation.
func (d *Definition) Detect(inv Invocation) Inputs {
	values := map[string]bool{}
	for _, flag := range d.Parsing.ValueFlags {
		values[flag] = true
	}
	for _, flag := range d.Passthrough.Flags {
		values[flag] = true
	}
	for _, f := range d.Fields {
		for _, flag := range f.Input.Flags {
			values[flag] = f.Type != "boolean"
		}
	}
	flags, positionals := parseArgs(inv.Args, values, d.Parsing)
	matches := func(input Input) bool {
		for _, flag := range input.Flags {
			if flags[flag] {
				return true
			}
		}
		for _, key := range input.Env {
			if envGet(inv.Env, key) != "" {
				return true
			}
		}
		for _, p := range input.Positionals {
			if p.Index >= len(positionals) {
				continue
			}
			if len(p.Prefixes) == 0 {
				return true
			}
			for _, prefix := range p.Prefixes {
				if strings.HasPrefix(positionals[p.Index], prefix) {
					return true
				}
			}
		}
		return false
	}
	result := Inputs{Passthrough: matches(d.Passthrough), Overrides: map[string]bool{}}
	for name, f := range d.Fields {
		if !matches(f.Input) {
			continue
		}
		if f.OnInput == "passthrough" {
			result.Passthrough = true
		} else {
			result.Overrides[name] = true
		}
	}
	return result
}

func parseArgs(args []string, values map[string]bool, parsing Parsing) (map[string]bool, []string) {
	flags := map[string]bool{}
	var positionals []string
	operands := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if operands {
			positionals = append(positionals, arg)
			continue
		}
		if arg == "--" {
			operands = true
			continue
		}
		if arg == "-" {
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			positionals = append(positionals, arg)
			if parsing.StopAtFirstPositional {
				operands = true
			}
			continue
		}
		name, _, attached := strings.Cut(arg, "=")
		if takesValue, known := values[name]; known {
			flags[name] = true
			if takesValue && !attached && i+1 < len(args) {
				i++
			}
			continue
		}
		if parsing.ShortOptions && !strings.HasPrefix(arg, "--") {
			for j := 1; j < len(arg); j++ {
				short := "-" + string(arg[j])
				takesValue, known := values[short]
				if known {
					flags[short] = true
				}
				if takesValue {
					if j == len(arg)-1 && i+1 < len(args) {
						i++
					}
					break
				}
			}
		}
	}
	return flags, positionals
}

// Value converts stored values as well as command-line strings. In particular,
// integer strings from version-1 Context files remain readable.
func (f Field) Value(raw any) (any, error) {
	if raw == nil {
		return nil, nil
	}
	var s string
	switch v := raw.(type) {
	case string:
		s = v
	case bool:
		if f.Type != "boolean" {
			return nil, fmt.Errorf("expected %s", f.Type)
		}
		s = strconv.FormatBool(v)
	case int:
		s = strconv.Itoa(v)
	case int64:
		s = strconv.FormatInt(v, 10)
	case float64:
		s = strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return nil, fmt.Errorf("expected %s", f.Type)
	}
	if strings.ContainsRune(s, 0) {
		return nil, fmt.Errorf("value cannot contain NUL")
	}
	if s == "" {
		return nil, nil
	}
	switch f.Type {
	case "string":
		if _, ok := raw.(string); !ok {
			return nil, fmt.Errorf("expected string")
		}
		if f.URI != nil {
			if _, err := applyURI(s, "", *f.URI); err != nil {
				return nil, err
			}
			if !f.Secret && uriHasPassword(s) {
				return nil, fmt.Errorf("URI containing a password requires secret: true")
			}
		}
		return s, nil
	case "integer":
		v, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			return v, nil
		}
	case "number":
		v, err := strconv.ParseFloat(s, 64)
		if err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) {
			return v, nil
		}
	case "boolean":
		if s == "true" {
			return true, nil
		}
		if s == "false" {
			return false, nil
		}
	}
	return nil, fmt.Errorf("expected %s", f.Type)
}

func (d *Definition) Activate(inv Invocation, ctx contextstore.Context, store secrets.Store) (Invocation, error) {
	inputs := d.Detect(inv)
	if inputs.Passthrough {
		return inv, nil
	}
	values := map[string]any{}
	for _, name := range d.FieldNames() {
		if inputs.Overrides[name] {
			continue
		}
		f := d.Fields[name]
		var raw any
		if f.Secret {
			if ref, ok := ctx.Secrets[name]; ok {
				if store == nil {
					return Invocation{}, fmt.Errorf("missing secret %s", name)
				}
				value, err := store.Get(ref)
				if err != nil {
					return Invocation{}, fmt.Errorf("missing secret %s", name)
				}
				raw = value
			} else if ctx.Metadata[name] != nil {
				return Invocation{}, fmt.Errorf("field %s must be configured in secret storage", name)
			}
		} else {
			if _, ok := ctx.Secrets[name]; ok {
				return Invocation{}, fmt.Errorf("field %s changed its secret storage policy; reconfigure it", name)
			}
			raw = ctx.Metadata[name]
		}
		v, err := f.Value(raw)
		if err != nil {
			return Invocation{}, fmt.Errorf("field %s: %w", name, err)
		}
		if v == nil && f.Required {
			return Invocation{}, fmt.Errorf("missing required field %s", name)
		}
		values[name] = v
	}
	result := Invocation{Env: append([]string(nil), inv.Env...)}
	var positional, flags []string
	for _, name := range d.FieldNames() {
		v := values[name]
		if v == nil {
			continue
		}
		f := d.Fields[name]
		if f.Type == "boolean" {
			if v == true && f.Inject.Flag != "" {
				flags = append(flags, f.Inject.Flag)
			}
			continue
		}
		value := fmt.Sprint(v)
		if f.URI != nil {
			path := ""
			if p := values[f.URI.DefaultPathFrom]; p != nil {
				path = fmt.Sprint(p)
			}
			var err error
			value, err = applyURI(value, path, *f.URI)
			if err != nil {
				return Invocation{}, fmt.Errorf("field %s: %w", name, err)
			}
		}
		if f.Inject.Positional {
			positional = append(positional, value)
		}
		if f.Inject.Flag != "" {
			flags = append(flags, f.Inject.Flag, value)
		}
		if f.Inject.Env != "" {
			result.Env = envSet(result.Env, f.Inject.Env, value)
		}
	}
	result.Args = append(positional, flags...)
	result.Args = append(result.Args, inv.Args...)
	return result, nil
}

// applyURI deliberately preserves multi-host authorities and existing paths.
// It does not use net/url's single-host parser for the authority.
func applyURI(raw, defaultPath string, rule URI) (string, error) {
	scheme, rest, ok := strings.Cut(raw, "://")
	allowed := false
	for _, candidate := range rule.Schemes {
		if scheme == candidate {
			allowed = true
		}
	}
	if !ok || !allowed {
		return "", fmt.Errorf("invalid URI scheme")
	}
	beforeQuery, query, hasQuery := strings.Cut(rest, "?")
	authority, path, _ := strings.Cut(beforeQuery, "/")
	if authority == "" || strings.ContainsAny(raw, "\r\n\t #") {
		return "", fmt.Errorf("invalid URI")
	}
	if path != "" || defaultPath == "" {
		return raw, nil
	}
	result := scheme + "://" + authority + "/" + url.PathEscape(defaultPath)
	if hasQuery {
		result += "?" + query
	}
	return result, nil
}

func uriHasPassword(raw string) bool {
	_, rest, _ := strings.Cut(raw, "://")
	authority, _, _ := strings.Cut(rest, "/")
	authority, _, _ = strings.Cut(authority, "?")
	userinfo, _, ok := strings.Cut(authority, "@")
	return ok && strings.Contains(userinfo, ":")
}

func envGet(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], key+"="); ok {
			return v
		}
	}
	return ""
}

func envSet(env []string, key, value string) []string {
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, key+"=") {
			result = append(result, entry)
		}
	}
	return append(result, key+"="+value)
}
