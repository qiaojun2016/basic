package oss

import (
	"encoding/base64"
	"encoding/json"
	"github.com/qiaojun2016/basic/id"
	"strings"
	"testing"
	"time"
)

func Test_server_GetURL(t *testing.T) {
	Server{
		Endpoint:        "",
		AccessKeyId:     "",
		AccessKeySecret: "",
		BucketName:      "",
	}.Run()
	url := Oss.GetURL("/dir/test.jpg")
	t.Log(url)
}

func initOss() {
	// 重置单例以便测试用不同配置初始化
	Oss = nil
	Server{
		Endpoint:        "https://oss-cn-hangzhou.aliyuncs.com",
		AccessKeyId:     "testAccessKeyId",
		AccessKeySecret: "testAccessKeySecret",
		BucketName:      "test-bucket",
	}.Run()
}

func TestPutSignPolicyFileIdURL_Fields(t *testing.T) {
	initOss()
	fId := "upload/images/avatar.jpg"

	result, err := Oss.PutSignPolicyFileIdURL(fId)
	if err != nil {
		t.Fatalf("PutSignPolicyFileIdURL 返回错误: %v", err)
	}
	t.Logf("返回结果: %+v", result)

	// 1. Url 格式：https://<bucket>.<endpoint去掉scheme>
	wantUrl := "https://test-bucket.oss-cn-hangzhou.aliyuncs.com"
	if result.Url != wantUrl {
		t.Errorf("Url = %q, want %q", result.Url, wantUrl)
	}

	// 2. OSSAccessKeyId 与初始化时一致
	if result.OSSAccessKeyId != "testAccessKeyId" {
		t.Errorf("OSSAccessKeyId = %q, want %q", result.OSSAccessKeyId, "testAccessKeyId")
	}

	// 3. Key 与传入 fId 一致
	if result.Key != fId {
		t.Errorf("Key = %q, want %q", result.Key, fId)
	}

	// 4. Signature 是合法的 base64
	sigBytes, err := base64.StdEncoding.DecodeString(result.Signature)
	if err != nil {
		t.Errorf("Signature 不是合法 base64: %v", err)
	}
	// SHA1 HMAC 输出固定 20 字节
	if len(sigBytes) != 20 {
		t.Errorf("Signature 解码后长度 = %d, want 20 (SHA1)", len(sigBytes))
	}
}

func TestPutSignPolicyFileIdURL_Policy(t *testing.T) {
	initOss()
	fId := "upload/docs/file.pdf"

	before := time.Now().UTC()
	result, err := Oss.PutSignPolicyFileIdURL(fId)
	after := time.Now().UTC()

	if err != nil {
		t.Fatalf("PutSignPolicyFileIdURL 返回错误: %v", err)
	}

	t.Logf("返回结果: %+v", result)

	// Policy 是合法的 base64
	policyBytes, err := base64.StdEncoding.DecodeString(result.Policy)
	if err != nil {
		t.Fatalf("Policy 不是合法 base64: %v", err)
	}
	t.Logf("Policy JSON: %s", string(policyBytes))

	// Policy 解析为 JSON
	var policy map[string]interface{}
	if err = json.Unmarshal(policyBytes, &policy); err != nil {
		t.Fatalf("Policy 不是合法 JSON: %v", err)
	}

	// expiration 字段存在
	expStr, ok := policy["expiration"].(string)
	if !ok {
		t.Fatal("policy 缺少 expiration 字段")
	}

	// expiration 格式正确，可解析为 UTC 时间
	exp, err := time.Parse("2006-01-02T15:04:05.000Z", expStr)
	if err != nil {
		t.Fatalf("expiration 格式解析失败 %q: %v", expStr, err)
	}

	// expiration 末尾必须是 Z（UTC 标识），不能是时区偏移
	if !strings.HasSuffix(expStr, "Z") {
		t.Errorf("expiration %q 不以 Z 结尾，时区可能错误", expStr)
	}

	// expiration 应在 [before+59s, after+61s] 范围内
	// 减 1s 宽容：expiration 格式截断到毫秒，before 带纳秒，边界可能差不足 1ms
	minExp := before.Add(59 * time.Second)
	maxExp := after.Add(61 * time.Second)
	if exp.Before(minExp) || exp.After(maxExp) {
		t.Errorf("expiration = %v，超出预期范围 [%v, %v]", exp, minExp, maxExp)
	}

	// conditions 包含 bucket 条件
	conditions, ok := policy["conditions"].([]interface{})
	if !ok || len(conditions) == 0 {
		t.Fatal("policy 缺少 conditions 字段")
	}
	foundBucket := false
	foundKey := false
	for _, c := range conditions {
		switch v := c.(type) {
		case map[string]interface{}:
			if v["bucket"] == "test-bucket" {
				foundBucket = true
			}
		case []interface{}:
			// ["eq", "$key", fId]
			if len(v) == 3 && v[0] == "eq" && v[1] == "$key" && v[2] == fId {
				foundKey = true
			}
		}
	}
	if !foundBucket {
		t.Errorf("conditions 中未找到 bucket = test-bucket")
	}
	if !foundKey {
		t.Errorf("conditions 中未找到 key = %q", fId)
	}
}

func TestUploadBase64(t *testing.T) {
	//id
	id.Server{
		Node: 1,
	}.Run()
	//
	Server{
		Endpoint:        "",
		AccessKeyId:     "",
		AccessKeySecret: "",
		BucketName:      "",
	}.Run()
	//
	url, err := Oss.UploadBase64("", "data:image/jpeg;base64,xxxxx")
	if err != nil {
		t.Error(err.Error())
		return
	}
	t.Log(url)
}
