package search

import "regexp"

// compileRE2 编译 RE2 语义正则（Go regexp 即 RE2，无回溯）。
func compileRE2(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile(pattern)
}
