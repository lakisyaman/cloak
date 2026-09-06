// Package connectors loads and executes declarative Connector definitions.
package connectors

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const MaxDefinitionBytes = 256 * 1024

type Definition struct {
	Version     int              `yaml:"version"`
	Command     string           `yaml:"command"`
	Description string           `yaml:"description,omitempty"`
	Fields      map[string]Field `yaml:"fields"`
	Passthrough Input            `yaml:"passthrough,omitempty"`
	Parsing     Parsing          `yaml:"parsing,omitempty"`
}

type Field struct {
	Type          string    `yaml:"type"`
	Required      bool      `yaml:"required,omitempty"`
	Secret        bool      `yaml:"secret,omitempty"`
	ConfigureFlag string    `yaml:"configureFlag,omitempty"`
	OnInput       string    `yaml:"onInput,omitempty"`
	Input         Input     `yaml:"input,omitempty"`
	Inject        Injection `yaml:"inject,omitempty"`
	URI           *URI      `yaml:"uri,omitempty"`
}

type Input struct {
	Flags       []string     `yaml:"flags,omitempty"`
	Env         []string     `yaml:"env,omitempty"`
	Positionals []Positional `yaml:"positionals,omitempty"`
}

type Positional struct {
	Index    int      `yaml:"index"`
	Prefixes []string `yaml:"prefixes,omitempty"`
}

type Injection struct {
	Flag       string `yaml:"flag,omitempty"`
	Env        string `yaml:"env,omitempty"`
	Positional bool   `yaml:"positional,omitempty"`
}

type URI struct {
	Schemes         []string `yaml:"schemes"`
	DefaultPathFrom string   `yaml:"defaultPathFrom,omitempty"`
}

type Parsing struct {
	// ValueFlags describe native options that do not bind Context fields.
	ValueFlags            []string `yaml:"valueFlags,omitempty"`
	ShortOptions          bool     `yaml:"shortOptions,omitempty"`
	StopAtFirstPositional bool     `yaml:"stopAtFirstPositional,omitempty"`
}

var commandPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var fieldPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*$`)
var configurePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var flagPattern = regexp.MustCompile(`^--?[a-zA-Z0-9][a-zA-Z0-9_-]*$`)
var envPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
var schemePattern = regexp.MustCompile(`^[a-z][a-z0-9+.-]*$`)

func ValidateCommand(name string) error {
	if !commandPattern.MatchString(name) || len(name) > 100 {
		return fmt.Errorf("invalid Managed CLI name")
	}
	switch name {
	case "cloak", "connector", "context", "shim", "doctor", "version", "help", "completion":
		return fmt.Errorf("Managed CLI name %s is reserved", name)
	}
	return nil
}

func Parse(data []byte) (*Definition, error) {
	if len(data) > MaxDefinitionBytes {
		return nil, fmt.Errorf("Connector definition exceeds size limit")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var def Definition
	if err := decoder.Decode(&def); err != nil {
		// YAML errors can quote values, including accidentally pasted credentials.
		return nil, fmt.Errorf("invalid Connector YAML (check keys, scalar types, and duplicate keys)")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("Connector must contain one YAML document")
	}
	if err := def.Validate(); err != nil {
		return nil, err
	}
	return &def, nil
}

func (d *Definition) FieldNames() []string {
	names := make([]string, 0, len(d.Fields))
	for name := range d.Fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (f Field) FlagName(name string) string {
	if f.ConfigureFlag != "" {
		return f.ConfigureFlag
	}
	return strings.ToLower(name)
}

func (i Input) empty() bool { return len(i.Flags)+len(i.Env)+len(i.Positionals) == 0 }

func (d *Definition) Validate() error {
	if d.Version != 1 {
		return fmt.Errorf("unsupported Connector version %d", d.Version)
	}
	if err := ValidateCommand(d.Command); err != nil {
		return err
	}
	if len(d.Fields) == 0 {
		return fmt.Errorf("Connector must declare fields")
	}
	inputs := map[string]bool{}
	checkInput := func(input Input) error {
		for _, flag := range input.Flags {
			if !flagPattern.MatchString(flag) || inputs["flag:"+flag] {
				return fmt.Errorf("invalid or repeated input flag")
			}
			inputs["flag:"+flag] = true
		}
		for _, env := range input.Env {
			if !envPattern.MatchString(env) || inputs["env:"+env] {
				return fmt.Errorf("invalid or repeated input environment variable")
			}
			inputs["env:"+env] = true
		}
		for _, p := range input.Positionals {
			key := fmt.Sprintf("position:%d", p.Index)
			if p.Index < 0 || inputs[key] {
				return fmt.Errorf("invalid or repeated positional input")
			}
			inputs[key] = true
			for _, prefix := range p.Prefixes {
				if prefix == "" || strings.ContainsAny(prefix, "\x00\r\n") {
					return fmt.Errorf("invalid positional prefix")
				}
			}
		}
		return nil
	}
	if err := checkInput(d.Passthrough); err != nil {
		return err
	}
	configFlags := map[string]bool{"help": true, "clear": true}
	injections := map[string]bool{}
	for _, name := range d.FieldNames() {
		f := d.Fields[name]
		if !fieldPattern.MatchString(name) {
			return fmt.Errorf("invalid field name")
		}
		switch f.Type {
		case "string", "integer", "number", "boolean":
		default:
			return fmt.Errorf("field %s has unsupported type", name)
		}
		if f.Secret && f.Type != "string" {
			return fmt.Errorf("secret field %s must be a string", name)
		}
		flag := f.FlagName(name)
		if !configurePattern.MatchString(flag) || configFlags[flag] {
			return fmt.Errorf("field %s has invalid or repeated configureFlag", name)
		}
		configFlags[flag] = true
		if !f.Input.empty() && f.OnInput != "passthrough" && f.OnInput != "override" {
			return fmt.Errorf("field %s needs onInput: passthrough or override", name)
		}
		if f.OnInput != "" && f.OnInput != "passthrough" && f.OnInput != "override" {
			return fmt.Errorf("field %s has unsupported onInput", name)
		}
		if err := checkInput(f.Input); err != nil {
			return fmt.Errorf("field %s: %w", name, err)
		}
		count, binding := 0, ""
		if f.Inject.Flag != "" {
			if !flagPattern.MatchString(f.Inject.Flag) {
				return fmt.Errorf("field %s has invalid injection flag", name)
			}
			count++
			binding = "flag:" + f.Inject.Flag
		}
		if f.Inject.Env != "" {
			if !envPattern.MatchString(f.Inject.Env) {
				return fmt.Errorf("field %s has invalid injection environment variable", name)
			}
			count++
			binding = "env:" + f.Inject.Env
		}
		if f.Inject.Positional {
			count++
			binding = "positional"
		}
		if count > 1 || binding != "" && injections[binding] {
			return fmt.Errorf("field %s has conflicting injection", name)
		}
		if binding != "" {
			injections[binding] = true
		}
		if f.Type == "boolean" && (f.Inject.Positional || f.Inject.Env != "") {
			return fmt.Errorf("boolean field %s must inject a flag", name)
		}
		if f.URI != nil {
			if f.Type != "string" || len(f.URI.Schemes) == 0 {
				return fmt.Errorf("field %s needs a string URI with schemes", name)
			}
			for _, scheme := range f.URI.Schemes {
				if !schemePattern.MatchString(scheme) {
					return fmt.Errorf("field %s has invalid URI scheme", name)
				}
			}
			if ref := f.URI.DefaultPathFrom; ref != "" {
				target, ok := d.Fields[ref]
				if !ok || ref == name || target.Type != "string" || target.Secret || target.URI != nil {
					return fmt.Errorf("field %s has invalid defaultPathFrom", name)
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, flag := range d.Parsing.ValueFlags {
		if !flagPattern.MatchString(flag) || seen[flag] {
			return fmt.Errorf("invalid or repeated parsing value flag")
		}
		seen[flag] = true
		for _, field := range d.Fields {
			if field.Type == "boolean" {
				for _, booleanFlag := range field.Input.Flags {
					if booleanFlag == flag {
						return fmt.Errorf("boolean input cannot consume a value")
					}
				}
			}
		}
	}
	return nil
}
