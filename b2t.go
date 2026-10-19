// b2tは10進数・2進数・16進数のバイト列とUTF-8テキストを相互変換するコマンドです。
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type options struct {
	reverse bool
	base    int
}

// parseBytesは各数値を1バイトとして読み取る。接頭辞なしは常に10進数。
// 負数を誤って正数として受け入れないよう、ハイフンは区切りにしない。
func parseBytes(s string) ([]byte, error) {
	tokens := strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || (r >= '\x1c' && r <= '\x1f') || r == ':' || r == ','
	})
	data := make([]byte, len(tokens))
	for i, token := range tokens {
		base := 10
		if len(token) >= 2 {
			switch token[:2] {
			case "0x", "0X", "0b", "0B":
				// 基数0では、Goの整数リテラルに従って_の位置も検証する。
				base = 0
			}
		}
		value, err := strconv.ParseUint(token, base, 8)
		if err != nil {
			if errors.Is(err, strconv.ErrRange) {
				return nil, fmt.Errorf("%d番目の値 %q が1バイトの範囲（0〜255）を超えています", i+1, token)
			}
			return nil, fmt.Errorf("%d番目の値 %q は不正です。10進数、0b付き2進数、0x付き16進数で指定してください", i+1, token)
		}
		data[i] = byte(value)
	}
	return data, nil
}

func textToNumbers(text string, base int) (string, error) {
	width := 0 // 接頭辞とバイト間の空白を含む、1バイト当たりの最大文字数。
	switch base {
	case 2:
		width = 11
	case 10:
		width = 4
	case 16:
		width = 5
	default:
		return "", errors.New("出力基数は2、10、16のいずれかで指定してください")
	}
	if !utf8.ValidString(text) {
		return "", errors.New("入力テキストが不正なUTF-8です")
	}
	n := len(text)
	if n == 0 {
		return "", nil
	}
	// 容量計算の整数オーバーフローを防ぐ。
	const maxInt = int(^uint(0) >> 1)
	if n > maxInt/width {
		return "", errors.New("入力テキストが大きすぎます")
	}

	var result strings.Builder
	result.Grow(width*n - 1)
	const hexDigits = "0123456789ABCDEF"
	for i := 0; i < n; i++ {
		if i > 0 {
			result.WriteByte(' ')
		}
		b := text[i]
		switch base {
		case 2:
			result.WriteString("0b")
			for bit := 7; bit >= 0; bit-- {
				result.WriteByte('0' + (b>>bit)&1)
			}
		case 10:
			result.WriteString(strconv.Itoa(int(b)))
		case 16:
			result.WriteString("0x")
			// 上位4ビット・下位4ビットをそれぞれ16進数1桁にする。
			result.WriteByte(hexDigits[b>>4])
			result.WriteByte(hexDigits[b&0x0F])
		}
	}
	return result.String(), nil
}

func convert(raw string, opts options) (string, error) {
	if opts.reverse {
		base := opts.base
		if base == 0 {
			base = 16
		}
		return textToNumbers(raw, base)
	}

	data, err := parseBytes(raw)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", errors.New("UTF-8として不正なバイト列です")
	}

	return strconv.Quote(string(data)), nil
}

// parseArgsは入力の前後にあるオプションを読み取る。
// -- 以降は、ハイフンで始まる文字列もすべて入力として扱う。
func parseArgs(args []string) (options, []string, error) {
	var opts options
	var inputs []string
	fs := flag.NewFlagSet("b2t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	fs.BoolVar(&opts.reverse, "r", false, "")
	fs.BoolVar(&opts.reverse, "reverse", false, "")
	var hex, dec, bin bool
	fs.BoolVar(&hex, "x", false, "")
	fs.BoolVar(&hex, "hex", false, "")
	fs.BoolVar(&dec, "d", false, "")
	fs.BoolVar(&dec, "dec", false, "")
	fs.BoolVar(&bin, "b", false, "")
	fs.BoolVar(&bin, "bin", false, "")

	for i, arg := range args {
		if arg == "--" {
			inputs = append(inputs, args[i+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			inputs = append(inputs, arg)
			continue
		}

		if strings.Contains(arg, "=") {
			return opts, nil, fmt.Errorf("オプション %q は値を取りません", arg)
		}
		flags := []string{arg}
		name := strings.TrimPrefix(arg, "-")
		// 既存の単一ハイフンの長い名前を優先し、残りは短縮フラグとして分解する。
		if !strings.HasPrefix(name, "-") && fs.Lookup(name) == nil && name != "help" {
			flags = nil
			for _, short := range name {
				flags = append(flags, "-"+string(short))
			}
		}
		// flagは最初の位置引数で解析を止めるため、各オプションを個別に渡す。
		for _, option := range flags {
			if err := fs.Parse([]string{option}); err != nil {
				return opts, nil, err
			}
		}
	}

	selected := 0
	for _, choice := range []struct {
		enabled bool
		base    int
	}{{hex, 16}, {dec, 10}, {bin, 2}} {
		if choice.enabled {
			selected++
			opts.base = choice.base
		}
	}
	if selected > 1 {
		return opts, nil, errors.New("出力基数は--hex、--dec、--binのいずれか1つを指定してください")
	}
	if selected > 0 && !opts.reverse {
		return opts, nil, errors.New("出力基数の指定には--reverseが必要です")
	}
	if opts.base == 0 {
		opts.base = 16
	}
	if opts.reverse && len(inputs) > 1 {
		return opts, nil, errors.New("--reverseの入力は1個までです。空白を含む文字列は引用符で囲んでください")
	}
	return opts, inputs, nil
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `数値のバイト列とUTF-8テキストを相互変換します。

Usage: b2t [OPTIONS] [BYTE]...
       b2t --reverse [--hex | --dec | --bin] [TEXT]

Arguments:
  [BYTE]...  変換するバイト値（0〜255）。異なる基数を混在できます。
             65 = 10進数 / 0b01000001 = 2進数 / 0x41 = 16進数
             接頭辞なしは常に10進数。0B / 0X も使用できます。
             区切りは空白・カンマ・コロン。
  [TEXT]     変換するテキスト。空白を含む場合は引用符で囲みます。

Options:
  -r, --reverse  テキストを数値のバイト列に変換（空白区切り）
  -x, --hex      16進数で出力（--reverse専用、既定。0x付き大文字2桁）
  -d, --dec      10進数で出力（--reverse専用、ゼロ埋めなし）
  -b, --bin      2進数で出力（--reverse専用、0b付き8桁）
  -h, --help     ヘルプを表示

  異なる出力基数は同時に指定できません。短縮フラグは-rbのように連結可能です。

Input:
  引数を省略すると、標準入力をEOFまで読み取ります。

Examples:
  b2t 65 0b01000010 0x43    # "ABC"
  b2t 0xE3 0x81 0x82        # "あ"
  b2t --reverse "ABC"       # 0x41 0x42 0x43
  b2t -rb "AB"             # 0b01000001 0b01000010
  b2t -rd "AB"             # 65 66
  printf '65 66 67' | b2t   # "ABC"
  b2t --reverse -- "-A"     # 0x2D 0x41
`)
}

func readInput(inputs []string, stdin io.Reader) (string, error) {
	if len(inputs) > 0 {
		return strings.Join(inputs, " "), nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", errors.New("標準入力のテキストはUTF-8で渡してください")
	}
	// 従来どおり、改行をLFに統一して末尾のLFを除去する。
	text := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(string(data))
	return strings.TrimRight(text, "\n"), nil
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, inputs, err := parseArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		printUsage(stdout)
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, "エラー:", err)
		return 2
	}

	raw, err := readInput(inputs, stdin)
	if err != nil {
		fmt.Fprintln(stderr, "エラー:", err)
		return 1
	}
	output, err := convert(raw, opts)
	if err != nil {
		fmt.Fprintln(stderr, "エラー:", err)
		return 1
	}
	if _, err := fmt.Fprintln(stdout, output); err != nil {
		fmt.Fprintln(stderr, "エラー:", err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
