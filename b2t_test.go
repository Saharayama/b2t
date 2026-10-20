package main

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"
	"testing/iotest"
	"unicode/utf8"
)

func TestParseBytes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []byte
	}{
		{"decimal", "65 66 67", []byte{65, 66, 67}},
		{"binary", "0b01000001 0B01000010", []byte{65, 66}},
		{"hexadecimal", "0x41 0X42", []byte{65, 66}},
		{"binary_underscores", "0b0100_0001 0B0100_0010", []byte{65, 66}},
		{"hex_underscores", "0x4_1 0X4_2 0xa_F 0xA_f", []byte{65, 66, 175, 175}},
		{"prefix_underscores", "0b_0100_0001 0B_0100_0010 0x_43 0X_4_4", []byte("ABCD")},
		{"arbitrary_digit_groups", "0b0_1_0 0x0_0_4_1", []byte{2, 65}},
		{"underscore_boundaries", "0b_0 0x0_0 0b1111_1111 0xF_F", []byte{0, 0, 255, 255}},
		{"underscores_with_separators", "0b0100_0001,0x4_2:67\t0X_44\n0B_0100_0101", []byte("ABCDE")},
		{"mixed_bases", "65 0b01000010 0x43", []byte("ABC")},
		{"separators", "65,66:67\t68\n69", []byte("ABCDE")},
		{"unicode_space", "65\u300066\x1c67", []byte("ABC")},
		{"boundaries", "0 255 0x00 0xFF 0b11111111", []byte{0, 255, 0, 255, 255}},
		{"leading_zero_is_decimal", "010", []byte{10}},
		{"empty", "", []byte{}},
		{"separators_only", " \n,:\t", []byte{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBytes(tt.input)
			if err != nil {
				t.Fatalf("parseBytes(%q): unexpected error: %v", tt.input, err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("parseBytes(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseBytesErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"negative", "-1", "1番目"},
		{"positive_sign", "+65", "不正"},
		{"decimal_out_of_range", "256", "範囲"},
		{"hex_out_of_range", "0x100", "範囲"},
		{"binary_out_of_range", "0b100000000", "範囲"},
		{"empty_hex_digits", "0x", "不正"},
		{"empty_binary_digits", "0b", "不正"},
		{"invalid_hex_digits", "0xGG", "不正"},
		{"invalid_binary_digits", "0b2", "不正"},
		{"unprefixed_hex", "FF", "不正"},
		{"unsupported_octal_prefix", "0o77", "不正"},
		{"hyphen_not_separator", "12-34", "不正"},
		{"error_position", "65,0xGG,67", "2番目"},
		{"leading_underscore", "_0x41", "不正"},
		{"underscore_in_hex_prefix", "0_x41", "不正"},
		{"underscore_in_binary_prefix", "0_b01000001", "不正"},
		{"trailing_hex_underscore", "0x41_", "不正"},
		{"trailing_binary_underscore", "0b01_", "不正"},
		{"consecutive_hex_underscores", "0x4__1", "不正"},
		{"consecutive_binary_underscores", "0b0__1", "不正"},
		{"consecutive_prefix_underscores", "0b__01", "不正"},
		{"hex_underscore_without_digits", "0x_", "不正"},
		{"binary_underscore_without_digits", "0b_", "不正"},
		{"hex_underscore_invalid_digit", "0x4_G", "不正"},
		{"binary_underscore_invalid_digit", "0b0_2", "不正"},
		{"decimal_underscore", "6_5", "不正"},
		{"leading_zero_decimal_underscore", "0_65", "不正"},
		{"octal_underscore", "0o7_7", "不正"},
		{"hex_underscore_out_of_range", "0x1_00", "範囲"},
		{"binary_underscore_out_of_range", "0b1_0000_0000", "範囲"},
		{"underscore_not_byte_separator", "0x41_42", "範囲"},
		{"underscore_error_position_and_token", "65,0x4__1,67", `2番目の値 "0x4__1" は不正`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBytes(tt.input)
			if err == nil {
				t.Fatalf("parseBytes(%q) = %v, want error containing %q", tt.input, got, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("parseBytes(%q) error = %q, want substring %q", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestTextToNumbersHex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ascii", "ABC", "0x41 0x42 0x43"},
		{"japanese", "あ", "0xE3 0x81 0x82"},
		{"emoji", "😀", "0xF0 0x9F 0x98 0x80"},
		{"space_and_newline", "A \n", "0x41 0x20 0x0A"},
		{"nul", "\x00", "0x00"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := textToNumbers(tt.input, 16)
			if err != nil {
				t.Fatalf("textToNumbers(%q, 16): unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("textToNumbers(%q, 16) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTextToNumbers(t *testing.T) {
	tests := []struct {
		name  string
		input string
		base  int
		want  string
	}{
		{"binary_ascii", "AB", 2, "0b01000001 0b01000010"},
		{"decimal_ascii", "AB", 10, "65 66"},
		{"binary_japanese", "あ", 2, "0b11100011 0b10000001 0b10000010"},
		{"decimal_japanese", "あ", 10, "227 129 130"},
		{"binary_emoji", "😀", 2, "0b11110000 0b10011111 0b10011000 0b10000000"},
		{"decimal_emoji", "😀", 10, "240 159 152 128"},
		{"binary_space_and_newline", "A \n", 2, "0b01000001 0b00100000 0b00001010"},
		{"decimal_space_and_newline", "A \n", 10, "65 32 10"},
		{"binary_nul", "\x00", 2, "0b00000000"},
		{"decimal_nul", "\x00", 10, "0"},
		{"binary_empty", "", 2, ""},
		{"decimal_empty", "", 10, ""},
		{"decimal_digit_widths", "\x01\x0a\x64", 10, "1 10 100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := textToNumbers(tt.input, tt.base)
			if err != nil || got != tt.want {
				t.Errorf("textToNumbers(%q, %d) = (%q, %v), want %q", tt.input, tt.base, got, err, tt.want)
			}
		})
	}
	for _, base := range []int{2, 10, 16} {
		if got, err := textToNumbers(string([]byte{0xff}), base); err == nil {
			t.Errorf("textToNumbers(invalid UTF-8, %d) = %q, want error", base, got)
		}
	}
	for _, base := range []int{0, 8, -1, 36} {
		for _, input := range []string{"", "A"} {
			if got, err := textToNumbers(input, base); err == nil {
				t.Errorf("textToNumbers(%q, %d) = %q, want error", input, base, got)
			}
		}
	}
}

func TestConvert(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		reverse bool
		want    string
		wantErr bool
	}{
		{"forward_ascii", "65 0x42 0b01000011", false, `"ABC"`, false},
		{"forward_underscores", "0b0100_0001 0x_4_2 0B_0100_0011", false, `"ABC"`, false},
		{"forward_japanese", "0xE3 0x81 0x82", false, `"あ"`, false},
		{"forward_escaped_newline", "65 10", false, `"A\n"`, false},
		{"forward_empty", "", false, `""`, false},
		{"forward_invalid_byte", "0xFF", false, "", true},
		{"forward_underscores_invalid_utf8", "0xF_F", false, "", true},
		{"forward_incomplete_utf8", "0xE3 0x81", false, "", true},
		{"forward_invalid_number", "256", false, "", true},
		{"reverse_ascii", "ABC", true, "0x41 0x42 0x43", false},
		{"reverse_underscore", "A_B", true, "0x41 0x5F 0x42", false},
		{"reverse_japanese", "あ", true, "0xE3 0x81 0x82", false},
		{"reverse_invalid_utf8", string([]byte{0xff}), true, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convert(tt.input, options{reverse: tt.reverse})
			if (err != nil) != tt.wantErr {
				t.Fatalf("convert(%q, reverse=%t) error = %v, wantErr %t", tt.input, tt.reverse, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("convert(%q, reverse=%t) = %q, want %q", tt.input, tt.reverse, got, tt.want)
			}
		})
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantReverse bool
		wantInputs  []string
		wantErr     bool
		wantHelp    bool
	}{
		{"no_args", nil, false, nil, false, false},
		{"multiple_inputs", []string{"65", "66"}, false, []string{"65", "66"}, false, false},
		{"short_flag_before", []string{"-r", "ABC"}, true, []string{"ABC"}, false, false},
		{"long_flag_after", []string{"ABC", "--reverse"}, true, []string{"ABC"}, false, false},
		{"double_dash", []string{"--reverse", "--", "-A"}, true, []string{"-A"}, false, false},
		{"double_dash_disables_flags", []string{"--", "-r", "65"}, false, []string{"-r", "65"}, false, false},
		{"lone_dash", []string{"-"}, false, []string{"-"}, false, false},
		{"reverse_two_inputs", []string{"-r", "A", "B"}, true, nil, true, false},
		{"unknown_flag", []string{"-unknown"}, false, nil, true, false},
		{"help", []string{"-h"}, false, nil, false, true},
		{"long_help", []string{"--help"}, false, nil, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, inputs, err := parseArgs(tt.args)
			if tt.wantHelp {
				if !errors.Is(err, flag.ErrHelp) {
					t.Fatalf("parseArgs(%q) error = %v, want flag.ErrHelp", tt.args, err)
				}
				return
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseArgs(%q) error = %v, wantErr %t", tt.args, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if opts.reverse != tt.wantReverse || !equalStrings(inputs, tt.wantInputs) {
				t.Errorf("parseArgs(%q) = (reverse=%t, inputs=%q), want (reverse=%t, inputs=%q)",
					tt.args, opts.reverse, inputs, tt.wantReverse, tt.wantInputs)
			}
			if opts.base != 16 {
				t.Errorf("default base = %d, want 16", opts.base)
			}
		})
	}
}

func TestParseArgsOutputBase(t *testing.T) {
	for _, choice := range []struct {
		base  int
		short string
		long  string
	}{{2, "b", "bin"}, {10, "d", "dec"}, {16, "x", "hex"}} {
		for _, args := range [][]string{
			{"-r", "-" + choice.short, "AB"},
			{"--reverse", "--" + choice.long, "AB"},
			{"--" + choice.long, "AB", "-r"},
			{"-r" + choice.short, "AB"},
			{"-" + choice.short + "r", "AB"},
			{"AB", "-r" + choice.short},
			{"-r", "-" + choice.short, "--" + choice.long, "AB"},
			{"-r" + choice.short + choice.short, "AB"},
		} {
			opts, inputs, err := parseArgs(args)
			if err != nil || !opts.reverse || opts.base != choice.base || !equalStrings(inputs, []string{"AB"}) {
				t.Errorf("parseArgs(%q) = (%+v, %q, %v), want reverse, base %d, input AB", args, opts, inputs, err, choice.base)
			}
		}
	}
	for _, args := range [][]string{
		{"-b", "AB"}, {"--dec", "AB"}, {"-x", "AB"},
		{"-rbd", "AB"}, {"-rdx", "AB"}, {"-rxb", "AB"},
		{"-r", "--bin", "--hex", "--bin", "AB"},
		{"-r", "--dec", "--bin", "AB"},
		{"-rb2", "AB"}, {"-r", "--bi", "AB"},
		{"-r", "--base", "2", "AB"},
		{"-rb", "A", "B"}, {"-rz", "AB"},
	} {
		if _, _, err := parseArgs(args); err == nil {
			t.Errorf("parseArgs(%q) should fail", args)
		}
	}
	for _, name := range []string{"r", "reverse", "x", "hex", "d", "dec", "b", "bin", "h", "help", "rb"} {
		for _, prefix := range []string{"-", "--"} {
			for _, value := range []string{"true", "false", "1", "0", "", "2"} {
				arg := prefix + name + "=" + value
				if _, _, err := parseArgs([]string{"-r", arg, "AB"}); err == nil || errors.Is(err, flag.ErrHelp) {
					t.Errorf("parseArgs with %q should reject option values, got %v", arg, err)
				}
			}
		}
	}
	for _, args := range [][]string{{"-reverse", "AB"}, {"--r", "AB"}} {
		if opts, _, err := parseArgs(args); err != nil || !opts.reverse {
			t.Errorf("existing alias %q failed: %v", args, err)
		}
	}
}

// nilと空スライスは入力一覧として同じ意味で扱う。
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReadInput(t *testing.T) {
	tests := []struct {
		name   string
		inputs []string
		stdin  string
		want   string
	}{
		{"from_args", []string{"65", "66"}, "ignored", "65 66"},
		{"from_stdin", nil, "65 66", "65 66"},
		{"crlf_normalized", nil, "A\r\nB\rC", "A\nB\nC"},
		{"trailing_newlines_removed", nil, "ABC\r\n\n", "ABC"},
		{"internal_newlines_preserved", nil, "A\nB", "A\nB"},
		{"empty_stdin", nil, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readInput(tt.inputs, strings.NewReader(tt.stdin))
			if err != nil {
				t.Fatalf("readInput: unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("readInput = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadInputErrors(t *testing.T) {
	t.Run("invalid_utf8_stdin", func(t *testing.T) {
		_, err := readInput(nil, bytes.NewReader([]byte{0xff}))
		if err == nil {
			t.Error("readInput(invalid UTF-8) should fail")
		}
	})
	t.Run("reader_error", func(t *testing.T) {
		_, err := readInput(nil, iotest.ErrReader(errors.New("read failed")))
		if err == nil || !strings.Contains(err.Error(), "read failed") {
			t.Errorf("readInput(reader error) = %v, want read failed", err)
		}
	})
	t.Run("args_ignore_reader", func(t *testing.T) {
		got, err := readInput([]string{"65"}, iotest.ErrReader(errors.New("should not read")))
		if err != nil || got != "65" {
			t.Errorf("readInput(args) = (%q, %v), want (65, nil)", got, err)
		}
	})
}

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		stdin      string
		wantCode   int
		wantStdout string
		wantStderr string // 空でない場合は部分一致で確認
	}{
		{"forward_args", []string{"65", "0x42", "0b01000011"}, "", 0, "\"ABC\"\n", ""},
		{"forward_stdin", nil, "65 66 67\r\n", 0, "\"ABC\"\n", ""},
		{"forward_underscore_args", []string{"0b0100_0001", "0x_4_2", "67"}, "", 0, "\"ABC\"\n", ""},
		{"forward_underscore_stdin", nil, "0b_0100_0001,0X4_2:67\r\n", 0, "\"ABC\"\n", ""},
		{"invalid_underscore_args", []string{"65", "0x4__1"}, "", 1, "", `2番目の値 "0x4__1" は不正`},
		{"invalid_underscore_stdin", nil, "0b01_\n", 1, "", "不正"},
		{"reverse_args", []string{"--reverse", "あ"}, "", 0, "0xE3 0x81 0x82\n", ""},
		{"reverse_stdin", []string{"-r"}, "ABC\r\n", 0, "0x41 0x42 0x43\n", ""},
		{"reverse_hex", []string{"-rx", "AB"}, "", 0, "0x41 0x42\n", ""},
		{"reverse_binary", []string{"-rb", "AB"}, "", 0, "0b01000001 0b01000010\n", ""},
		{"reverse_decimal", []string{"--reverse", "--dec", "AB"}, "", 0, "65 66\n", ""},
		{"reverse_binary_stdin", []string{"-br"}, "あ\r\n", 0, "0b11100011 0b10000001 0b10000010\n", ""},
		{"reverse_decimal_stdin", []string{"-rd"}, "😀\n", 0, "240 159 152 128\n", ""},
		{"reverse_binary_empty", []string{"-rb", ""}, "ignored", 0, "\n", ""},
		{"reverse_decimal_empty", []string{"-rd"}, "", 0, "\n", ""},
		{"reverse_numeric_text", []string{"-rb", "2"}, "", 0, "0b00110010\n", ""},
		{"reverse_false_text", []string{"-r", "false"}, "", 0, "0x66 0x61 0x6C 0x73 0x65\n", ""},
		{"literal_flag_text", []string{"-rd", "--", "-r=false"}, "", 0, "45 114 61 102 97 108 115 101\n", ""},
		{"literal_cluster_text", []string{"-rd", "--", "-rb2"}, "", 0, "45 114 98 50\n", ""},
		{"base_requires_reverse", []string{"--bin", "AB"}, "", 2, "", "--reverseが必要"},
		{"conflicting_bases", []string{"-rbd", "AB"}, "", 2, "", "いずれか1つ"},
		{"reverse_flag_value", []string{"-r=false", "65"}, "", 2, "", "値を取りません"},
		{"invalid_attached_base", []string{"-rb2", "AB"}, "", 2, "", "エラー:"},
		{"binary_invalid_utf8", []string{"-rb", string([]byte{0xff})}, "", 1, "", "UTF-8"},
		{"empty_input", nil, "", 0, "\"\"\n", ""},
		{"invalid_number", []string{"256"}, "", 1, "", "範囲"},
		{"invalid_utf8_bytes", []string{"0xFF"}, "", 1, "", "UTF-8"},
		{"invalid_flag", []string{"-wrong"}, "", 2, "", "エラー:"},
		{"invalid_reverse_args", []string{"-r", "A", "B"}, "", 2, "", "入力は1個まで"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, strings.NewReader(tt.stdin), &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if got := stdout.String(); got != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", got, tt.wantStdout)
			}
			if got := stderr.String(); !strings.Contains(got, tt.wantStderr) || (tt.wantStderr == "" && got != "") {
				t.Errorf("stderr = %q, want substring %q", got, tt.wantStderr)
			}
		})
	}
}

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "-rh", "-rbh"} {
		t.Run(arg, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{arg}, strings.NewReader(""), &stdout, &stderr)
			if code != 0 || !strings.Contains(stdout.String(), "Usage:\n  b2t") || !strings.Contains(stdout.String(), "--bin") || stderr.Len() != 0 {
				t.Errorf("help: code=%d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunIOErrors(t *testing.T) {
	t.Run("stdin_read_error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run(nil, iotest.ErrReader(errors.New("read failed")), &stdout, &stderr)
		if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "read failed") {
			t.Errorf("code=%d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
		}
	})
	t.Run("stdin_invalid_utf8", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run(nil, bytes.NewReader([]byte{0xff}), &stdout, &stderr)
		if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "UTF-8") {
			t.Errorf("code=%d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
		}
	})
	t.Run("stdout_write_error", func(t *testing.T) {
		var stderr bytes.Buffer
		code := run([]string{"65"}, strings.NewReader(""), failingWriter{errors.New("write failed")}, &stderr)
		if code != 1 || !strings.Contains(stderr.String(), "write failed") {
			t.Errorf("code=%d, stderr=%q", code, stderr.String())
		}
	})
}

// 任意の有効なUTF-8テキストが各基数のバイト列との往復で失われないことを確認する。
func FuzzRoundTrip(f *testing.F) {
	for _, seed := range []string{"", "ABC", "あいうえお", "😀", "A\x00B", "A\r\nB", string([]byte{0xff})} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		for _, base := range []int{2, 10, 16} {
			numbers, err := textToNumbers(input, base)
			if !utf8.ValidString(input) {
				if err == nil {
					t.Errorf("textToNumbers accepted invalid UTF-8 input %q", input)
				}
				continue
			}
			if err != nil {
				t.Fatalf("textToNumbers(%q, %d): %v", input, base, err)
			}
			data, err := parseBytes(numbers)
			if err != nil {
				t.Fatalf("parseBytes(%q): %v", numbers, err)
			}
			if got := string(data); got != input {
				t.Errorf("round trip = %q, want %q", got, input)
			}
		}
	})
}

// -rオプションのテキストから16進数への変換性能を測定する。
func BenchmarkTextToHex(b *testing.B) {
	input := strings.Repeat("あいうえおA", 1000)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := textToNumbers(input, 16); err != nil {
			b.Fatal(err)
		}
	}
}

// 標準出力への書き込み失敗を模擬する。
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }
