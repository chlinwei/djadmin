package assets

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"autoadmin/internal/shared/apperror"
)

var templatePortMacro = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}$`)

// TemplatePortValue preserves macro expressions while keeping literal ports numeric on the wire.
type TemplatePortValue string

func (port *TemplatePortValue) UnmarshalJSON(raw []byte) error {
	var value string
	if len(raw) > 0 && raw[0] == '"' {
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
	} else {
		var number json.Number
		if err := json.Unmarshal(raw, &number); err != nil {
			return err
		}
		value = number.String()
	}
	*port = TemplatePortValue(strings.TrimSpace(value))
	return nil
}

func (port TemplatePortValue) MarshalJSON() ([]byte, error) {
	value := string(port)
	if number, err := strconv.Atoi(value); err == nil && number >= 1 && number <= 65535 {
		return json.Marshal(number)
	}
	return json.Marshal(value)
}

func validateTemplatePorts(ports *[]TemplatePortInput) error {
	if ports == nil {
		return nil
	}
	for index, item := range *ports {
		value := strings.TrimSpace(string(item.Port))
		if templatePortMacro.MatchString(value) && len(value) <= 64 {
			continue
		}
		number, err := strconv.Atoi(value)
		if err != nil || number < 1 || number > 65535 || strconv.Itoa(number) != value {
			return apperror.New(apperror.CodeInvalidArgument,
				fmt.Sprintf("第 %d 个端口必须为 1–65535 的整数或 ${PORT} 格式的宏（最长 64 字符）", index+1))
		}
	}
	return nil
}
