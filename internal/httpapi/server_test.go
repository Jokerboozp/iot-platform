package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestVideoSignature(t *testing.T) {
	body := []byte(`{"eventId":"1"}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte("123"))
	_, _ = mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))
	if !verifySignature("secret", "123", body, signature) {
		t.Fatal("valid signature rejected")
	}
	if verifySignature("secret", "123", body, "bad") {
		t.Fatal("bad signature accepted")
	}
}

func TestFrontendManagementControls(t *testing.T) {
	app, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	index, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	assets := string(app) + string(index)
	for _, label := range []string{"查看告警详情", "查看详情", "下载", "批量下载", "raw-select-all", "未注册设备", "一键注册", "所属网关", "网关自动注册子设备", "手动添加规则", "保存规则", "data-rule-edit", "data-rule-delete", "接入配置已保存"} {
		if !strings.Contains(assets, label) {
			t.Errorf("frontend is missing %q", label)
		}
	}
}

func TestFrontendUsesChineseEnumLabels(t *testing.T) {
	app, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	index, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	assets := string(app) + string(index)
	for _, label := range []string{"火灾风险", "检测到火焰", "紧急", "活动中", "设备上报", "疑似离线", "数据静默", "已发布", "烟雾探测器", "边缘网关"} {
		if !strings.Contains(assets, label) {
			t.Errorf("frontend is missing Chinese enum label %q", label)
		}
	}
	for _, visibleEnglish := range []string{">ACTIVE<", ">ACKED<", ">CRITICAL<", ">HIGH<", ">ENABLED<", ">DISABLED<", ">PUBLISHED<"} {
		if strings.Contains(string(index), visibleEnglish) {
			t.Errorf("frontend still exposes English enum option %q", visibleEnglish)
		}
	}
}
