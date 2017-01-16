package gtpm

import "bytes"
import "fmt"
import "io"
import "strconv"
import "strings"

type (
	// Matcher is the interface that tries to match given Reader against a rule
	Matcher interface {
		// MatchReader returns matched if given Reader match a rule
		MatchReader(io.Reader) (matched [][]byte, err error)
	}
	// TextPatternMatcher implements Matcher with Text Pattern Matching(DSL)
	TextPatternMatcher struct {
		instSlice  []instruction
		intBinds   []int
		maxVarSize int
	}
	// ErrorCode includes an error description.
	ErrorCode string
	// Error holds information related to an error.
	Error struct {
		// Code is the description of this error.
		Code ErrorCode
		// Pos is where this error occurred.
		Pos int
		// Cause is set to an error if this error caused by some other error.
		Cause error
	}
	// Option defines a functional parameter.
	Option      func(*TextPatternMatcher)
	instruction func(io.Reader) ([]byte, error)
	parseState  int
)

const (
	defaultInstCap    = 8
	defaultMaxVarSize = 4096
)

const (
	ErrConstNotMuch     = "gtpm: const not matched"
	ErrVarNotMuch       = "gtpm: variable not matched"
	ErrVarExceedMaxSize = "gtpm: variable size exceeded the maximum: %d"
	ErrIntVarNotMuch    = "gtpm: integer variable not matched"
)

const (
	ErrParseColonExpected      = "gtpm: parse error. ':' expected"
	ErrParseVariableNotDefined = "gtpm: parse error. variable: %s not defined"
	ErrParseSuffixExpected     = "gtpm: parse error. suffix expected"
	ErrParseInvalidSlash       = "gtpm: parse error. '/' appeared more than onece"
	ErrParseInvalidType        = "gtpm: parse error. \"bin\" or \"int\" should appear after '/'"
)

const (
	nonParseState parseState = iota
	blindParseState
	binParseState
	intParseState
)

func (e Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s at %d caused by %+v", e.Code, e.Pos, e.Cause)
	}
	return fmt.Sprintf("%s at %d", e.Code, e.Pos)
}

func WithMaxVariableSize(max int) Option {
	return func(tpm *TextPatternMatcher) {
		tpm.maxVarSize = max
	}
}

func Compile(pattern string, opts ...Option) (Matcher, error) {
	matcher := &TextPatternMatcher{}
	for _, opt := range opts {
		opt(matcher)
	}
	if matcher.instSlice == nil {
		matcher.instSlice = make([]instruction, 0, defaultInstCap)
	}
	if matcher.maxVarSize == 0 {
		matcher.maxVarSize = defaultMaxVarSize
	}
	r := bytes.NewBufferString(pattern)
	intBindsMap := make(map[string]int)
	var state parseState
	pos := 1
	var name string
	for {
		rawLine, err := r.ReadString(',')
		if err != nil && err != io.EOF {
			return nil, err
		}
		// trim the last ','
		var line string
		if rawLine[len(rawLine)-1] == ',' {
			line = rawLine[:len(rawLine)-1]
		} else {
			line = rawLine
		}
		// 1. blind(unbind) (start with '_')
		//   - "_" # the subsequent block must be const
		//   - "_:12"
		//   - "_:Number" # Number is an integer variable
		// 2. bind binary variable
		//   - "var/bin" # the subsequent block must be const
		//   - "var/bin:12"
		//   - "var/bin:Number" # Number is an integer variable
		// 3. bind integer variable
		//   - "var/int" # the subsequent block must be const
		//   - "var/int:12"
		//   - "var/int:Number" # Number is an integer variable
		// 4. const (arbitrary bytes: not matched with any rule)
		//   - suffix for the above types
		//     - "_, suffix"
		//     - "var/bin, suffix"
		//     - "var/int, suffix"
		//   - or pure const
		if line[0] == '_' {
			// blind
			if len(line) == 1 {
				// "_"
				state = blindParseState
			} else {
				tokens := strings.Split(line, ":")
				if len(tokens) != 2 {
					return nil, Error{Code: ErrParseColonExpected, Pos: pos}
				}
				n, err := strconv.ParseInt(tokens[1], 10, 64)
				if err == nil {
					// "_:12"
					matcher.intBinds = append(matcher.intBinds, int(n))
					matcher.instSlice = append(matcher.instSlice, genInstVarWithSize(pos, &matcher.intBinds[len(matcher.intBinds)-1], false))
				} else {
					// "_:Number"
					idx, ok := intBindsMap[tokens[1]]
					if !ok {
						return nil, Error{Code: ErrorCode(fmt.Sprintf(ErrParseVariableNotDefined, tokens[1])), Pos: pos}
					}
					matcher.instSlice = append(matcher.instSlice, genInstVarWithSize(pos, &matcher.intBinds[idx], false))
				}
			}
		} else if strings.Contains(line, "/") {
			// bind binary|integer
			tokens := strings.Split(line, "/")
			if len(tokens) != 2 {
				return nil, Error{Code: ErrParseInvalidSlash, Pos: pos}
			}
			if len(tokens[1]) < 3 {
				return nil, Error{Code: ErrParseInvalidType, Pos: pos}
			}
			switch tokens[1][:3] {
			case "bin":
				subTokens := strings.Split(tokens[1], ":")
				if subTokens[0] != "bin" {
					return nil, Error{Code: ErrParseInvalidType, Pos: pos}
				}
				if len(subTokens) == 2 {
					n, err := strconv.ParseInt(subTokens[1], 10, 64)
					if err == nil {
						//   - "var/bin:12"
						matcher.intBinds = append(matcher.intBinds, int(n))
						matcher.instSlice = append(matcher.instSlice, genInstVarWithSize(pos, &matcher.intBinds[len(matcher.intBinds)-1], true))
					} else {
						//   - "var/bin:Number"
						idx, ok := intBindsMap[subTokens[1]]
						if !ok {
							return nil, Error{Code: ErrorCode(fmt.Sprintf(ErrParseVariableNotDefined, subTokens[1])), Pos: pos}
						}
						matcher.instSlice = append(matcher.instSlice, genInstVarWithSize(pos, &matcher.intBinds[idx], true))
					}
				} else {
					//   - "var/bin"
					state = binParseState
				}
			case "int":
				subTokens := strings.Split(tokens[1], ":")
				if subTokens[0] != "int" {
					return nil, Error{Code: ErrParseInvalidType, Pos: pos}
				}
				if len(subTokens) == 2 {
					n, err := strconv.ParseInt(subTokens[1], 10, 64)
					if err == nil {
						//   - "var/int:12"
						matcher.intBinds = append(matcher.intBinds, int(n))
						matcher.intBinds = append(matcher.intBinds, 0)
						intBindsMap[tokens[0]] = len(matcher.intBinds) - 1
						matcher.instSlice = append(matcher.instSlice, genInstIntWithSize(pos, &matcher.intBinds[len(matcher.intBinds)-2], &matcher.intBinds[len(matcher.intBinds)-1]))
					} else {
						//   - "var/int:Number"
						idx, ok := intBindsMap[subTokens[1]]
						if !ok {
							return nil, Error{Code: ErrorCode(fmt.Sprintf(ErrParseVariableNotDefined, subTokens[1])), Pos: pos}
						}
						matcher.intBinds = append(matcher.intBinds, 0)
						intBindsMap[tokens[0]] = len(matcher.intBinds) - 1
						matcher.instSlice = append(matcher.instSlice, genInstIntWithSize(pos, &matcher.intBinds[idx], &matcher.intBinds[len(matcher.intBinds)-1]))
					}
				} else {
					//   - "var/int"
					name = tokens[0]
					state = intParseState
				}
			default:
				return nil, Error{Code: ErrParseInvalidType, Pos: pos}
			}
		} else if state != nonParseState {
			// suffix for blind/binary|integer
			switch state {
			case blindParseState:
				// blind
				// "_, suffix"
				matcher.instSlice = append(matcher.instSlice, genInstVarWithoutSize(pos, []byte(line), false, matcher.maxVarSize))
			case binParseState:
				// binary
				// "var/bin, suffix"
				matcher.instSlice = append(matcher.instSlice, genInstVarWithoutSize(pos, []byte(line), true, matcher.maxVarSize))
			case intParseState:
				// integer
				// "var/int, suffix"
				matcher.intBinds = append(matcher.intBinds, 0)
				intBindsMap[name] = len(matcher.intBinds) - 1
				matcher.instSlice = append(matcher.instSlice, genInstIntWithoutSize(pos, []byte(line), &matcher.intBinds[len(matcher.intBinds)-1], matcher.maxVarSize))
			}
			state = nonParseState
		} else {
			// pure const
			matcher.instSlice = append(matcher.instSlice, genInstConst(pos, []byte(line)))
		}
		if err == io.EOF {
			if state != nonParseState {
				return nil, Error{Code: ErrParseSuffixExpected, Pos: pos}
			}
			return matcher, nil
		}
		pos += len(rawLine)
	}
}

func (tpm *TextPatternMatcher) MatchReader(r io.Reader) (matched [][]byte, err error) {
	var binds [][]byte
	for _, inst := range tpm.instSlice {
		buf, err := inst(r)
		if err != nil {
			return nil, err
		}
		if buf != nil {
			binds = append(binds, buf)
		}
	}
	return binds, nil
}

func genInstConst(pos int, match []byte) instruction {
	return func(r io.Reader) ([]byte, error) {
		l := len(match)
		buf := make([]byte, l)
		for i := 0; i < l; {
			n, err := r.Read(buf[i:])
			if err != nil {
				return nil, Error{Code: ErrConstNotMuch, Pos: pos, Cause: err}
			}
			i += n
		}
		if !bytes.Equal(match, buf) {
			return nil, Error{Code: ErrConstNotMuch, Pos: pos}
		}
		return nil, nil
	}
}

func genInstVarWithSize(pos int, size *int, capture bool) instruction {
	return func(r io.Reader) ([]byte, error) {
		buf := make([]byte, *size)
		for i := 0; i < *size; {
			n, err := r.Read(buf[i:])
			if err != nil {
				return nil, Error{Code: ErrVarNotMuch, Pos: pos, Cause: err}
			}
			i += n
		}
		if capture {
			return buf, nil
		} else {
			return nil, nil
		}
	}
}

func genInstVarWithoutSize(pos int, suffix []byte, capture bool, max int) instruction {
	return func(r io.Reader) ([]byte, error) {
		var idx int
		var midx int
		bs := 16
		buf := make([]byte, bs)
		for {
			_, err := r.Read(buf[idx : idx+1])
			if err != nil {
				return nil, Error{Code: ErrVarNotMuch, Pos: pos, Cause: err}
			}
			idx++
			if idx >= len(suffix) {
				if bytes.Equal(suffix, buf[midx:midx+len(suffix)]) {
					if capture {
						return buf[:midx], nil
					} else {
						return nil, nil
					}
				}
				midx++
			}
			if idx == bs {
				// extend buf
				bs *= 2
				if bs > max {
					return nil, Error{Code: ErrorCode(fmt.Sprintf(ErrVarExceedMaxSize, max)), Pos: pos}
				}
				new := make([]byte, bs)
				copy(new, buf)
				buf = new
			}
		}
	}
}

func genInstIntWithSize(pos int, size *int, outSize *int) instruction {
	return func(r io.Reader) ([]byte, error) {
		buf := make([]byte, *size)
		for i := 0; i < *size; {
			n, err := r.Read(buf[i:])
			if err != nil {
				return nil, Error{Code: ErrIntVarNotMuch, Pos: pos, Cause: err}
			}
			i += n
		}
		n, err := strconv.ParseInt(string(buf), 10, 64)
		if err != nil {
			return nil, Error{Code: ErrIntVarNotMuch, Pos: pos, Cause: err}
		}
		*outSize = int(n)
		return buf, nil
	}
}

func genInstIntWithoutSize(pos int, suffix []byte, outSize *int, max int) instruction {
	return func(r io.Reader) ([]byte, error) {
		var idx int
		var midx int
		bs := 16
		buf := make([]byte, bs)
		for {
			_, err := r.Read(buf[idx : idx+1])
			if err != nil {
				return nil, Error{Code: ErrIntVarNotMuch, Pos: pos, Cause: err}
			}
			idx++
			if idx >= len(suffix) {
				if bytes.Equal(suffix, buf[midx:midx+len(suffix)]) {
					n, err := strconv.ParseInt(string(buf[:midx]), 10, 64)
					if err != nil {
						return nil, Error{Code: ErrIntVarNotMuch, Pos: pos, Cause: err}
					}
					*outSize = int(n)
					return buf[:midx], nil
				}
				midx++
			}
			if idx == bs {
				// extend buf
				bs *= 2
				if bs > max {
					return nil, Error{Code: ErrorCode(fmt.Sprintf(ErrVarExceedMaxSize, max)), Pos: pos}
				}
				new := make([]byte, bs)
				copy(new, buf)
				buf = new
			}
		}
	}
}
