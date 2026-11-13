package aptos

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidMoveType is returned for malformed Move type tags.
var ErrInvalidMoveType = errors.New("invalid Move type")

const (
	maxTypeDepth  = 16
	maxTypeLength = 1024
)

var primitiveTypes = map[string]struct{}{
	"bool":    {},
	"u8":      {},
	"u16":     {},
	"u32":     {},
	"u64":     {},
	"u128":    {},
	"u256":    {},
	"address": {},
	"signer":  {},
}

// Type is a parsed Move type tag. Exactly one shape is populated:
//   - Vector != nil: vector<Vector>
//   - Module != "":   address::module::name with optional type arguments
//   - Name != "":     primitive type
type Type struct {
	Address  string
	Module   string
	Name     string
	TypeArgs []Type
	Vector   *Type
}

// String returns the canonical form of the type tag.
func (t Type) String() string {
	if t.Vector != nil {
		return "vector<" + t.Vector.String() + ">"
	}
	if t.Module != "" {
		out := t.Address + "::" + t.Module + "::" + t.Name
		if len(t.TypeArgs) > 0 {
			args := make([]string, len(t.TypeArgs))
			for i, arg := range t.TypeArgs {
				args[i] = arg.String()
			}
			out += "<" + strings.Join(args, ", ") + ">"
		}
		return out
	}
	return t.Name
}

// IsPrimitive reports whether the type is a Move primitive.
func (t Type) IsPrimitive() bool {
	_, ok := primitiveTypes[t.Name]
	return t.Vector == nil && t.Module == "" && ok
}

// ParseType parses a Move type tag into a canonical form. Nested type
// arguments and vectors are supported up to a bounded depth.
func ParseType(value string) (Type, error) {
	if len(value) == 0 || len(value) > maxTypeLength {
		return Type{}, fmt.Errorf("%w: type must be 1-%d bytes", ErrInvalidMoveType, maxTypeLength)
	}
	parser := typeParser{input: value}
	parsed, err := parser.parseType(0)
	if err != nil {
		return Type{}, err
	}
	parser.skipSpace()
	if parser.pos != len(parser.input) {
		return Type{}, fmt.Errorf("%w: unexpected trailing input at %q", ErrInvalidMoveType, parser.input[parser.pos:])
	}
	return parsed, nil
}

// NormalizeType parses and returns the canonical type string.
func NormalizeType(value string) (string, error) {
	parsed, err := ParseType(value)
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}

type typeParser struct {
	input string
	pos   int
}

func (p *typeParser) parseType(depth int) (Type, error) {
	if depth > maxTypeDepth {
		return Type{}, fmt.Errorf("%w: nesting exceeds %d", ErrInvalidMoveType, maxTypeDepth)
	}
	p.skipSpace()
	start := p.pos
	identifier := p.readToken()
	if identifier == "" {
		return Type{}, fmt.Errorf("%w: expected identifier at byte %d", ErrInvalidMoveType, start)
	}
	p.skipSpace()
	if p.peek() == '<' {
		if identifier != "vector" {
			return Type{}, fmt.Errorf("%w: generics require a struct tag, got %q", ErrInvalidMoveType, identifier)
		}
		p.pos++
		element, err := p.parseType(depth + 1)
		if err != nil {
			return Type{}, err
		}
		p.skipSpace()
		if p.peek() != '>' {
			return Type{}, fmt.Errorf("%w: expected '>' after vector element", ErrInvalidMoveType)
		}
		p.pos++
		return Type{Vector: &element}, nil
	}
	if p.peek() != ':' {
		if _, ok := primitiveTypes[identifier]; !ok {
			return Type{}, fmt.Errorf("%w: unknown primitive %q", ErrInvalidMoveType, identifier)
		}
		return Type{Name: identifier}, nil
	}
	if p.pos+1 >= len(p.input) || p.input[p.pos+1] != ':' {
		return Type{}, fmt.Errorf("%w: expected '::' after address", ErrInvalidMoveType)
	}
	p.pos += 2
	address, err := ParseAddress(identifier)
	if err != nil {
		return Type{}, fmt.Errorf("%w: %v", ErrInvalidMoveType, err)
	}
	p.skipSpace()
	module := p.readIdentifier()
	if module == "" {
		return Type{}, fmt.Errorf("%w: expected module name", ErrInvalidMoveType)
	}
	p.skipSpace()
	if p.pos+1 >= len(p.input) || p.input[p.pos] != ':' || p.input[p.pos+1] != ':' {
		return Type{}, fmt.Errorf("%w: expected '::' after module", ErrInvalidMoveType)
	}
	p.pos += 2
	p.skipSpace()
	name := p.readIdentifier()
	if name == "" {
		return Type{}, fmt.Errorf("%w: expected struct name", ErrInvalidMoveType)
	}
	out := Type{Address: address, Module: module, Name: name}
	p.skipSpace()
	if p.peek() == '<' {
		p.pos++
		for {
			arg, err := p.parseType(depth + 1)
			if err != nil {
				return Type{}, err
			}
			out.TypeArgs = append(out.TypeArgs, arg)
			p.skipSpace()
			switch p.peek() {
			case ',':
				p.pos++
			case '>':
				p.pos++
				return out, nil
			default:
				return Type{}, fmt.Errorf("%w: expected ',' or '>' in type arguments", ErrInvalidMoveType)
			}
		}
	}
	return out, nil
}

func (p *typeParser) readToken() string {
	start := p.pos
	for p.pos < len(p.input) {
		char := p.input[p.pos]
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' {
			p.pos++
			continue
		}
		break
	}
	return p.input[start:p.pos]
}

func (p *typeParser) readIdentifier() string {
	start := p.pos
	token := p.readToken()
	if token == "" {
		return ""
	}
	first := token[0]
	if first >= '0' && first <= '9' {
		p.pos = start
		return ""
	}
	return token
}

func (p *typeParser) skipSpace() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *typeParser) peek() byte {
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}
