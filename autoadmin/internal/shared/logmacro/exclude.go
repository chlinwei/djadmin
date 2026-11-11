package logmacro

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

// DirectoryPattern converts a single-level directory glob to the RE2 expression
// used both by file discovery and Filebeat's filestream scanner.
func DirectoryPattern(directory string) (string, error) {
	if directory == "" || directory != strings.TrimSpace(directory) || !strings.HasPrefix(directory, "/") || strings.ContainsAny(directory, "\r\n\\") || strings.Contains(directory, "**") || strings.Contains(directory, "${") {
		return "", fmt.Errorf("排除目录必须是已展开宏的绝对路径，不支持 **、反斜线或换行：%s", directory)
	}
	directory = strings.TrimSuffix(directory, "/")
	if directory == "" {
		return "", fmt.Errorf("排除目录不能是根目录")
	}
	for _, segment := range strings.Split(directory, "/")[1:] {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("排除目录不能包含空路径段、. 或 ..：%s", directory)
		}
	}
	if _, err := path.Match(directory, ""); err != nil {
		return "", fmt.Errorf("排除目录通配符无效：%s", directory)
	}
	var expression strings.Builder
	expression.WriteString("^")
	for index := 0; index < len(directory); index++ {
		switch directory[index] {
		case '*':
			expression.WriteString("[^/]*")
		case '?':
			expression.WriteString("[^/]")
		case '[':
			end := index + 1
			for end < len(directory) && directory[end] != ']' {
				end++
			}
			if end == len(directory) || strings.Contains(directory[index:end], "/") {
				return "", fmt.Errorf("排除目录字符组无效：%s", directory)
			}
			class := directory[index+1 : end]
			if strings.HasPrefix(class, "^") {
				expression.WriteString("[^/")
				class = strings.TrimPrefix(class, "^")
			} else {
				expression.WriteString("[")
			}
			expression.WriteString(class)
			expression.WriteString("]")
			index = end
		default:
			character, size := utf8.DecodeRuneInString(directory[index:])
			expression.WriteString(regexp.QuoteMeta(string(character)))
			index += size - 1
		}
	}
	expression.WriteString("/")
	if _, err := regexp.Compile(expression.String()); err != nil {
		return "", fmt.Errorf("排除目录字符组无效：%s", directory)
	}
	return expression.String(), nil
}

func ValidateExcludeDirectories(directories []string) error {
	for _, directory := range directories {
		// Macro syntax is checked separately; substitute a safe path component only
		// while validating the directory glob before instance variables are known.
		candidate := regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}`).ReplaceAllString(directory, "macro")
		if strings.HasPrefix(directory, "${") {
			candidate = "/" + candidate
		}
		if _, err := DirectoryPattern(candidate); err != nil {
			return err
		}
	}
	return nil
}

func ResolveExcludeDirectories(directories []string, macroSets ...map[string]string) ([]string, error) {
	patterns := make([]string, 0, len(directories))
	for _, directory := range directories {
		resolved, err := ResolveChecked(directory, macroSets...)
		if err != nil {
			return nil, err
		}
		pattern, err := DirectoryPattern(resolved)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

func ValidateExcludeFilePatterns(patterns []string) error {
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" || strings.ContainsAny(pattern, "\r\n") {
			return fmt.Errorf("排除文件正则不能为空或包含换行")
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("排除文件正则无效 %q：%w", pattern, err)
		}
	}
	return nil
}

// File expressions are literal RE2 patterns against the full path, just like
// Filebeat exclude_files; macro expansion must not rewrite their regex syntax.
func ResolveExcludeJSON(raw, filePatterns json.RawMessage, macroSets ...map[string]string) ([]string, error) {
	var directories []string
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &directories); err != nil {
			return nil, fmt.Errorf("排除目录必须是字符串数组：%w", err)
		}
	}
	patterns, err := ResolveExcludeDirectories(directories, macroSets...)
	if err != nil {
		return nil, err
	}
	var files []string
	if len(filePatterns) != 0 {
		if err := json.Unmarshal(filePatterns, &files); err != nil {
			return nil, fmt.Errorf("排除文件正则必须是字符串数组：%w", err)
		}
	}
	if err := ValidateExcludeFilePatterns(files); err != nil {
		return nil, err
	}
	return append(patterns, files...), nil
}
