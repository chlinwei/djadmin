package monitor

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 告警媒介测试发送（POST /monitor/media/:id/test/）。
// 与 Django AlertMediaViewSet.test 行为一致：仅支持 Email 媒介，同步发信、不落库，
// 发信逻辑与真实告警通知共用（后续通知功能也走 sendSMTPMedia）。

type alertMediaTestInput struct {
	Recipients any    `json:"recipients"`
	Subject    string `json:"subject"`
	Message    string `json:"message"`
}

// parseAlertMediaTestRecipients 对齐 Django：接受数组或逗号/分号分隔字符串。
func parseAlertMediaTestRecipients(raw any) []string {
	switch typed := raw.(type) {
	case string:
		return splitRecipients(typed)
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			for _, recipient := range splitRecipients(fmt.Sprint(item)) {
				items = append(items, recipient)
			}
		}
		return items
	default:
		return nil
	}
}

func splitRecipients(value string) []string {
	items := make([]string, 0)
	for _, part := range strings.Split(strings.ReplaceAll(value, ";", ","), ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func (handler *Handler) TestAlertMedia(context *gin.Context) {
	id := parseID(context.Param("id"))
	row, err := db.New(handler.db).GetAlertMediaTyped(context, id)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "alert media not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	if row.MediaType != "email" {
		response.BusinessError(context, 400, "当前仅支持测试 Email 媒介", nil)
		return
	}
	var input alertMediaTestInput
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	recipients := parseAlertMediaTestRecipients(input.Recipients)
	if len(recipients) == 0 {
		response.BusinessError(context, 400, "请至少填写一个收件人", nil)
		return
	}
	if strings.TrimSpace(input.Subject) == "" {
		response.BusinessError(context, 400, "请填写主题", nil)
		return
	}
	if strings.TrimSpace(input.Message) == "" {
		response.BusinessError(context, 400, "请填写消息", nil)
		return
	}

	var config map[string]any
	_ = json.Unmarshal(row.Config, &config)
	if success, errorMessage := handler.sendSMTPMedia(config, input.Subject, input.Message, recipients); !success {
		response.BusinessError(context, 400, fmt.Sprintf("测试邮件发送失败：%s", errorMessage), nil)
		return
	}
	response.Success(context, gin.H{"sent": true})
}

// sendSMTPMedia 对齐 Django monitor.tasks.send_smtp_email：
// smtpPort 缺省 587；gmail 强制 STARTTLS；useSSL 走隐式 TLS（如 465）；
// messageFormat=html 时以 multipart/alternative 同时携带纯文本与 HTML 正文。
// 返回 (success, error_message)，错误文案以"邮件发送失败: "开头，与 Django 拼接格式一致。
func (handler *Handler) sendSMTPMedia(config map[string]any, subject, body string, recipients []string) (bool, string) {
	config = configOrEmpty(config)
	host := strings.TrimSpace(stringValue(config["smtpServer"]))
	port := defaultString(config["smtpPort"], "587")
	from := strings.TrimSpace(stringValue(config["email"]))
	username := strings.TrimSpace(stringValue(config["username"]))
	password := ""
	if encrypted := stringValue(config["password"]); encrypted != "" {
		decrypted, err := handler.secrets.Decrypt(encrypted)
		if err != nil {
			return false, fmt.Sprintf("邮件发送失败: %v", err)
		}
		password = decrypted
	}
	useTLS := configBool(config["useTLS"]) || strings.EqualFold(strings.TrimSpace(stringValue(config["provider"])), "gmail")
	useSSL := configBool(config["useSSL"])
	messageFormat := defaultString(config["messageFormat"], "text")

	addr := net.JoinHostPort(host, port)
	if host == "" {
		return false, "邮件发送失败: smtpServer 未配置"
	}
	if from == "" {
		return false, "邮件发送失败: 发件人邮箱未配置"
	}
	tlsConfig := &tls.Config{ServerName: host}
	var client *smtp.Client
	var err error
	if useSSL {
		var connection net.Conn
		connection, err = tls.DialWithDialer(&net.Dialer{Timeout: 20 * time.Second}, "tcp", addr, tlsConfig)
		if err == nil {
			client, err = smtp.NewClient(connection, host)
		}
	} else {
		var connection net.Conn
		connection, err = net.DialTimeout("tcp", addr, 20*time.Second)
		if err == nil {
			client, err = smtp.NewClient(connection, host)
		}
		if err == nil && useTLS {
			err = client.StartTLS(tlsConfig)
		}
	}
	if err != nil {
		return false, fmt.Sprintf("邮件发送失败: %v", err)
	}
	defer client.Close()

	if username != "" {
		if ok, _ := client.Extension("AUTH"); ok {
			if err = client.Auth(smtp.PlainAuth("", username, password, host)); err != nil {
				return false, fmt.Sprintf("邮件发送失败: %v", err)
			}
		}
	}
	if err = client.Mail(from); err != nil {
		return false, fmt.Sprintf("邮件发送失败: %v", err)
	}
	for _, recipient := range recipients {
		if err = client.Rcpt(recipient); err != nil {
			return false, fmt.Sprintf("邮件发送失败: %v", err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return false, fmt.Sprintf("邮件发送失败: %v", err)
	}
	_, writeErr := writer.Write(buildSMTPMessage(from, recipients, subject, body, messageFormat))
	closeErr := writer.Close()
	quitErr := client.Quit()
	if writeErr != nil {
		return false, fmt.Sprintf("邮件发送失败: %v", writeErr)
	}
	if closeErr != nil {
		return false, fmt.Sprintf("邮件发送失败: %v", closeErr)
	}
	if quitErr != nil {
		return false, fmt.Sprintf("邮件发送失败: %v", quitErr)
	}
	return true, ""
}

// buildSMTPMessage 组装 RFC 5322 报文；主题按 RFC 2047 编码以支持中文。
func buildSMTPMessage(from string, recipients []string, subject, body, messageFormat string) []byte {
	encodedSubject := mime.QEncoding.Encode("UTF-8", subject)
	header := &strings.Builder{}
	header.WriteString("From: " + from + "\r\n")
	header.WriteString("To: " + strings.Join(recipients, ", ") + "\r\n")
	header.WriteString("Subject: " + encodedSubject + "\r\n")
	header.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	if messageFormat == "html" {
		boundary := "djadmin-alert-boundary"
		header.WriteString("MIME-Version: 1.0\r\n")
		header.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")
		header.WriteString("--" + boundary + "\r\n")
		header.WriteString("Content-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
		header.WriteString(body + "\r\n\r\n")
		header.WriteString("--" + boundary + "\r\n")
		header.WriteString("Content-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
		header.WriteString(body + "\r\n\r\n")
		header.WriteString("--" + boundary + "--\r\n")
		return []byte(header.String())
	}
	header.WriteString("MIME-Version: 1.0\r\n")
	header.WriteString("Content-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	header.WriteString(body + "\r\n")
	return []byte(header.String())
}

func configOrEmpty(config map[string]any) map[string]any {
	if config == nil {
		return map[string]any{}
	}
	return config
}

func configBool(value any) bool {
	typed, ok := value.(bool)
	return ok && typed
}
