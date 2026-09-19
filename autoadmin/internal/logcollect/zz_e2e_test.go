package logcollect

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"autoadmin/internal/assets"

	_ "github.com/go-sql-driver/mysql"
)

func envValue(key string) string {
	file, _ := os.Open("../../config.env")
	if file == nil {
		file, _ = os.Open("config.env")
	}
	if file == nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, key+"=") {
			return strings.Trim(strings.TrimPrefix(line, key+"="), `"'`)
		}
	}
	return ""
}

// 现场样例（用户 2026-09-19 提供）：Spring Boot 默认控制台格式 + Oracle 连接异常堆栈。
const realSample = `2026-09-19 19:29:50.158 ERROR 3229551 --- [eate-1106216507] com.alibaba.druid.pool.DruidDataSource   : create connection SQLException, url: jdbc:oracle:thin:@//192.168.201.207:1521/acdm, errorCode 12514, state 08006

java.sql.SQLRecoverableException: Listener refused the connection with the following error:
ORA-12514, TNS:listener does not currently know of service requested in connect descriptor
 
        at oracle.jdbc.driver.T4CConnection.logon(T4CConnection.java:774)
        at oracle.jdbc.driver.PhysicalConnection.connect(PhysicalConnection.java:688)
        at com.alibaba.druid.pool.DruidAbstractDataSource.createPhysicalConnection(DruidAbstractDataSource.java:1703)
        at com.alibaba.druid.pool.DruidDataSource$CreateConnectionThread.run(DruidDataSource.java:2946)
Caused by: oracle.net.ns.NetException: Listener refused the connection with the following error:
ORA-12514, TNS:listener does not currently know of service requested in connect descriptor
 
        at oracle.net.ns.NSProtocolNIO.negotiateConnection(NSProtocolNIO.java:271)
        at oracle.net.ns.NSProtocol.connect(NSProtocol.java:317)
        ... 6 common frames omitted
2026-09-19 19:29:51.001 INFO 3229551 --- [main] com.example.Foo : 下一条正常记录`

func TestE2ERealSample(t *testing.T) {
	dsn := envValue("MYSQL_DSN")
	if dsn == "" {
		t.Skip("读不到 MYSQL_DSN")
	}
	db, _ := sql.Open("mysql", dsn)
	defer db.Close()
	var startPattern, body string
	var multiline bool
	if err := db.QueryRow(`SELECT start_pattern, multiline_enabled, pipeline_body
		FROM monitor_log_processing_rule WHERE name = 'springboot-tomcat-exception'`).
		Scan(&startPattern, &multiline, &body); err != nil {
		t.Fatalf("%v", err)
	}
	var pipeline map[string]any
	_ = json.Unmarshal([]byte(body), &pipeline)

	// 模拟反向读取窗口：开头再切一段"上一条记录的残尾"
	tail := "\tat com.example.Cut(From.java:1)\n" + realSample

	docs, err := logSampleDocs(tail, multiline, startPattern)
	if err != nil {
		t.Fatalf("还原记录失败：%v", err)
	}
	fmt.Printf("还原出 %d 条记录：\n", len(docs))
	for index, doc := range docs {
		message := doc.(map[string]any)["message"].(string)
		fmt.Printf("  --- 记录 %d（%d 行，%d 字节）---\n%s\n", index+1, len(strings.Split(message, "\n")), len(message), message)
	}

	var hosts, user, encrypted string
	_ = db.QueryRow("SELECT hosts, username, password FROM monitor_elasticsearch_cluster WHERE enabled = TRUE ORDER BY is_default DESC, id LIMIT 1").
		Scan(&hosts, &user, &encrypted)
	encryptor, _ := assets.NewSecretEncryptor("", envValue("JWT_SECRET"))
	password, _ := encryptor.Decrypt(encrypted)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	wrapped := []any{}
	for _, doc := range docs {
		wrapped = append(wrapped, map[string]any{"_source": doc})
	}
	payload, _ := json.Marshal(map[string]any{"pipeline": pipeline, "docs": wrapped})
	request, _ := http.NewRequest("POST", hosts+"/_ingest/pipeline/_simulate", bytes.NewReader(payload))
	request.SetBasicAuth(user, password)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%v", err)
	}
	raw, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("HTTP %d %s", response.StatusCode, string(raw))
	}
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	for index, rawDoc := range result["docs"].([]any) {
		source := rawDoc.(map[string]any)["doc"].(map[string]any)["_source"].(map[string]any)
		appFields, _ := source["app_fields"].(map[string]any)
		fmt.Printf("\n记录 %d → log_level=%v @timestamp=%v\n  log_message=%.60v\n  error_fingerprint=%v\n  app_fields=%v\n",
			index+1, source["log_level"], source["@timestamp"], source["log_message"], source["error_fingerprint"], appFields)
	}
	if missing := missingRequiredDocumentFields(result); len(missing) > 0 {
		t.Fatalf("认证判定不通过，缺：%v", missing)
	}
	fmt.Println("\n认证判定：通过（必备字段齐）")
}

// TestE2ELegacyRenameIsRejectedOnRealCluster 现场回归（kul 的 tomcat，2026-09-19）：
// 把规则改回**当时的样子**（首位多一个 `rename log → message`，Fluent Bit 时代的遗留），
// 用真实样例在真集群上跑 —— 必须被判"不通过"。
//
// 这条用例存在的意义：那套写法在"样例只有 message"的旧口径下**永远判通过**，而主机上每一条事件
// 都被 ES 以 400 拒收。它把"校验必须能看见真实载荷造成的破坏"钉在真集群语义上（不是 mock 假设）。
func TestE2ELegacyRenameIsRejectedOnRealCluster(t *testing.T) {
	dsn := envValue("MYSQL_DSN")
	if dsn == "" {
		t.Skip("读不到 MYSQL_DSN")
	}
	database, _ := sql.Open("mysql", dsn)
	defer database.Close()
	var startPattern, body string
	var multiline bool
	if err := database.QueryRow(`SELECT start_pattern, multiline_enabled, pipeline_body
		FROM monitor_log_processing_rule WHERE name = 'springboot-tomcat-exception'`).
		Scan(&startPattern, &multiline, &body); err != nil {
		t.Fatalf("%v", err)
	}
	var pipeline map[string]any
	if err := json.Unmarshal([]byte(body), &pipeline); err != nil {
		t.Fatalf("解析 pipeline 失败：%v", err)
	}
	// 把它改回现场形态。
	legacy := map[string]any{"rename": map[string]any{
		"field": "log", "ignore_missing": true, "override": true, "target_field": "message",
	}}
	pipeline["processors"] = append([]any{legacy}, pipeline["processors"].([]any)...)

	var hosts, user, encrypted string
	var prefix string
	_ = database.QueryRow("SELECT hosts, username, password, index_prefix FROM monitor_elasticsearch_cluster WHERE enabled = TRUE ORDER BY is_default DESC, id LIMIT 1").
		Scan(&hosts, &user, &encrypted, &prefix)
	encryptor, _ := assets.NewSecretEncryptor("", envValue("JWT_SECRET"))
	password, _ := encryptor.Decrypt(encrypted)

	docs, err := logSampleDocs(realSample, multiline, startPattern)
	if err != nil {
		t.Fatalf("还原记录失败：%v", err)
	}
	handler := &Handler{}
	context := ginContextForTest()
	cluster := elasticsearchCluster{Hosts: hosts, Username: user, Password: password, IndexPrefix: prefix, Timeout: 10}
	result, err := handler.simulatePipeline(context, cluster, "", pipeline, docs)
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	missing, _ := result["missing_fields"].([]string)
	if len(missing) == 0 {
		t.Fatal("现场形态的规则（rename log→message）必须被判不通过——否则平台又会给出「全绿」的假信号")
	}
	hint := clobberedMessageHint(result, realSample)
	if !strings.Contains(hint, "rename log") {
		t.Fatalf("应指出 message 被搬成了对象，得到 %q", hint)
	}
	fmt.Printf("现场形态判定：不通过（缺 %v）；%s\n", missing, hint)
}
