package gtpm

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"testing"
)

func checkError(got error, want error) bool {
	if got != nil && want != nil {
		// Error:Error
		return got.(Error).Error() == want.(Error).Error()
	}
	// nil:nil
	// nil:Error
	// Error:nil
	return got == want
}

func invokeInst(inst instruction, r io.Reader, wantBuf []byte, wantErr error, t *testing.T) {
	ret, err := inst(r)
	if !bytes.Equal(ret, wantBuf) || !checkError(err, wantErr) {
		t.Errorf("gtpm_test: got %#v, %+v, want %#v, %+v", ret, err, wantBuf, wantErr)
	}
}

func cmpByteSliceSlice(a [][]byte, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i, ae := range a {
		if !bytes.Equal(ae, b[i]) {
			return false
		}
	}
	return true
}

func TestGenInstConst(t *testing.T) {
	tests := []struct {
		read []byte
		src  []byte
		pos  int
		want []byte
		err  error
	}{
		{
			read: []byte("foo"),
			src:  []byte("foo"),
			pos:  0,
			want: nil,
			err:  nil,
		},
		{
			read: []byte("foo"),
			src:  []byte("bar"),
			pos:  1, want: nil,
			err: Error{Code: ErrConstNotMuch, Pos: 1},
		},
		{
			read: []byte("foo"),
			src:  []byte("buzz"),
			pos:  2, want: nil,
			err: Error{Code: ErrConstNotMuch, Pos: 2, Cause: io.EOF},
		},
	}
	for _, test := range tests {
		r := bytes.NewReader(test.read)
		inst := genInstConst(test.pos, test.src)
		invokeInst(inst, r, test.want, test.err, t)
	}

}

func TestGenInstVarWithSize(t *testing.T) {
	tests := []struct {
		read    []byte
		pos     int
		size    int
		capture bool
		want    []byte
		err     error
	}{
		{
			read:    []byte("foo"),
			pos:     0,
			size:    3,
			capture: true,
			want:    []byte("foo"),
			err:     nil,
		},
		{
			read:    []byte("foo"),
			pos:     1,
			size:    3,
			capture: false,
			want:    nil,
			err:     nil,
		},
		{
			read:    []byte("foo"),
			pos:     2,
			size:    4,
			capture: true,
			want:    nil,
			err:     Error{Code: ErrVarNotMuch, Pos: 2, Cause: io.EOF},
		},
	}
	for _, test := range tests {
		r := bytes.NewReader(test.read)
		inst := genInstVarWithSize(test.pos, &test.size, test.capture)
		invokeInst(inst, r, test.want, test.err, t)
	}

}

func TestGenInstVarWithoutSize(t *testing.T) {
	tests := []struct {
		read    []byte
		pos     int
		suffix  []byte
		capture bool
		max     int
		want    []byte
		err     error
	}{
		{
			read:    []byte("foobar"),
			pos:     0,
			suffix:  []byte("bar"),
			capture: true,
			max:     1024,
			want:    []byte("foo"),
			err:     nil,
		},
		{
			read:    []byte("foobar"),
			pos:     1,
			suffix:  []byte("bar"),
			capture: false,
			max:     1024,
			want:    nil,
			err:     nil,
		},
		{
			read:    []byte("foobar"),
			pos:     2,
			suffix:  []byte("buzz"),
			capture: true,
			max:     1024,
			want:    nil,
			err:     Error{Code: ErrVarNotMuch, Pos: 2, Cause: io.EOF},
		},
		{
			read:    []byte("foobarfoobarfoobarbuzz"),
			pos:     3,
			suffix:  []byte("buzz"),
			capture: true,
			max:     1024,
			want:    []byte("foobarfoobarfoobar"),
			err:     nil,
		},
		{
			read:    []byte("foobarfoobarfoobarbuzz"),
			pos:     4,
			suffix:  []byte("buzz"),
			capture: true,
			max:     16,
			want:    nil,
			err:     Error{Code: ErrorCode(fmt.Sprintf(ErrVarExceedMaxSize, 16)), Pos: 4},
		},
	}
	for _, test := range tests {
		r := bytes.NewReader(test.read)
		inst := genInstVarWithoutSize(test.pos, test.suffix, test.capture, test.max)
		invokeInst(inst, r, test.want, test.err, t)
	}

}

func TestGenInstIntWithSize(t *testing.T) {
	tests := []struct {
		read []byte
		pos  int
		size int
		out  int
		want []byte
		err  error
	}{
		{
			read: []byte("123"),
			pos:  0,
			size: 3,
			out:  0,
			want: []byte("123"),
			err:  nil,
		},
		{
			read: []byte("foo"),
			pos:  1,
			size: 3,
			out:  0,
			want: nil,
			err:  Error{Code: ErrIntVarNotMuch, Pos: 1, Cause: &strconv.NumError{Func: "ParseInt", Num: "foo", Err: strconv.ErrSyntax}},
		},
		{
			read: []byte("foo"),
			pos:  2,
			size: 4,
			out:  0,
			want: nil,
			err:  Error{Code: ErrIntVarNotMuch, Pos: 2, Cause: io.EOF},
		},
	}
	for _, test := range tests {
		r := bytes.NewReader(test.read)
		inst := genInstIntWithSize(test.pos, &test.size, &test.out)
		invokeInst(inst, r, test.want, test.err, t)
		if test.out != 0 {
			n, _ := strconv.ParseInt(string(test.want), 10, 64)
			if test.out != int(n) {
				t.Errorf("gtpm_test: got %d, want %d", test.out, n)
			}
		}
	}

}

func TestGenInstIntWithoutSize(t *testing.T) {
	tests := []struct {
		read   []byte
		pos    int
		suffix []byte
		out    int
		max    int
		want   []byte
		err    error
	}{
		{
			read:   []byte("789bar"),
			pos:    0,
			suffix: []byte("bar"),
			out:    0,
			max:    1024,
			want:   []byte("789"),
			err:    nil,
		},
		{
			read:   []byte("foobar"),
			pos:    1,
			suffix: []byte("bar"),
			out:    0,
			max:    1024,
			want:   nil,
			err:    Error{Code: ErrIntVarNotMuch, Pos: 1, Cause: &strconv.NumError{Func: "ParseInt", Num: "foo", Err: strconv.ErrSyntax}},
		},
		{
			read:   []byte("foobar"),
			pos:    2,
			suffix: []byte("buzz"),
			out:    0,
			max:    1024,
			want:   nil,
			err:    Error{Code: ErrIntVarNotMuch, Pos: 2, Cause: io.EOF},
		},
		{
			read:   []byte("1234567890foobarbuzz"),
			pos:    3,
			suffix: []byte("foobarbuzz"),
			out:    0,
			max:    1024,
			want:   []byte("1234567890"),
			err:    nil,
		},
		{
			read:   []byte("1234567890foobarbuzz"),
			pos:    4,
			suffix: []byte("foobarbuzz"),
			out:    0,
			max:    16,
			want:   nil,
			err:    Error{Code: ErrorCode(fmt.Sprintf(ErrVarExceedMaxSize, 16)), Pos: 4},
		},
	}
	for _, test := range tests {
		r := bytes.NewReader(test.read)
		inst := genInstIntWithoutSize(test.pos, test.suffix, &test.out, test.max)
		invokeInst(inst, r, test.want, test.err, t)
		if test.out != 0 {
			n, _ := strconv.ParseInt(string(test.want), 10, 64)
			if test.out != int(n) {
				t.Errorf("gtpm_test: got %d, want %d", test.out, n)
			}
		}
	}

}

func TestCompileAndMatch(t *testing.T) {
	tests := []struct {
		pattern string
		read    []byte
		cerr    error
		want    [][]byte
		merr    error
		opts    []Option
	}{
		{
			pattern: "N/int,\r\n",
			read:    []byte("123\r\n"),
			cerr:    nil,
			want: [][]byte{
				[]byte("123"),
			},
			merr: nil,
		},
		{
			pattern: "_,\r\n",
			read:    []byte("deadbeaf\r\n"),
			cerr:    nil,
			want:    nil,
			merr:    nil,
		},
		{
			pattern: "_:4",
			read:    []byte("dead"),
			cerr:    nil,
			want:    nil,
			merr:    nil,
		},
		{
			pattern: "N/int,\r\n,_:N",
			read:    []byte("4\r\nbeaf"),
			cerr:    nil,
			want: [][]byte{
				[]byte("4"),
			},
			merr: nil,
		},
		{
			pattern: "N/int,\r\n,_:N:0",
			read:    nil,
			cerr:    Error{Code: ErrParseColonExpected, Pos: 10},
			want:    nil,
			merr:    nil,
		},
		{
			pattern: "N/int,\r\n,_:M",
			read:    nil,
			cerr:    Error{Code: ErrorCode(fmt.Sprintf(ErrParseVariableNotDefined, "M")), Pos: 10},
			want:    nil,
			merr:    nil,
		},
		{
			pattern: "Number/int,\r\n,_:Number",
			read:    []byte("4\r\nbea"),
			cerr:    nil,
			want:    nil,
			merr:    Error{Code: ErrVarNotMuch, Pos: 15, Cause: io.EOF},
		},
		{
			pattern: "V/bin,\r\n,N/int:2,v2/bin:N,\r\n",
			read:    []byte("foobarbuzz\r\n16abcdef0123456789\r\n"),
			cerr:    nil,
			want: [][]byte{
				[]byte("foobarbuzz"),
				[]byte("16"),
				[]byte("abcdef0123456789"),
			},
			merr: nil,
			opts: []Option{WithMaxVariableSize(32)},
		},
		{
			pattern: "V/bin:3,N/int:1,\t,N2/int:N,var/bin:N2",
			read:    []byte("abc1\t8deadbeaf"),
			cerr:    nil,
			want: [][]byte{
				[]byte("abc"),
				[]byte("1"),
				[]byte("8"),
				[]byte("deadbeaf"),
			},
			merr: nil,
		},
		{
			pattern: "N/int",
			read:    nil,
			cerr:    Error{Code: ErrParseSuffixExpected, Pos: 1},
			want:    nil,
			merr:    nil,
		},
		{
			pattern: "N/int/bin",
			read:    nil,
			cerr:    Error{Code: ErrParseInvalidSlash, Pos: 1},
			want:    nil,
			merr:    nil,
		},
		{
			pattern: "hoge,N/bi",
			read:    nil,
			cerr:    Error{Code: ErrParseInvalidType, Pos: 6},
			want:    nil,
			merr:    nil,
		},
		{
			pattern: "foo,N/inte",
			read:    nil,
			cerr:    Error{Code: ErrParseInvalidType, Pos: 5},
			want:    nil,
			merr:    nil,
		},
		{
			pattern: "foo,N/binary",
			read:    nil,
			cerr:    Error{Code: ErrParseInvalidType, Pos: 5},
			want:    nil,
			merr:    nil,
		},
		{
			pattern: "foo,N/foo",
			read:    nil,
			cerr:    Error{Code: ErrParseInvalidType, Pos: 5},
			want:    nil,
			merr:    nil,
		},
		{
			pattern: "N/int,\r\n,foo/int:M",
			read:    nil,
			cerr:    Error{Code: ErrorCode(fmt.Sprintf(ErrParseVariableNotDefined, "M")), Pos: 10},
			want:    nil,
			merr:    nil,
		},
		{
			pattern: "N/int,\r\n,foo/bin:Num",
			read:    nil,
			cerr:    Error{Code: ErrorCode(fmt.Sprintf(ErrParseVariableNotDefined, "Num")), Pos: 10},
			want:    nil,
			merr:    nil,
		},
	}
	for _, test := range tests {
		m, err := Compile(test.pattern, test.opts...)
		if err != test.cerr {
			t.Errorf("gtpm_test: got %+v, want %+v", err, test.cerr)
		}
		if err != nil {
			continue
		}
		r := bytes.NewReader(test.read)
		matched, err := m.MatchReader(r)
		if !cmpByteSliceSlice(matched, test.want) || err != test.merr {
			t.Errorf("gtpm_test: got %#v %+v, want %#v %+v", matched, err, test.want, test.merr)
		}
	}
}
