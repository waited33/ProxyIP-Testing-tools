package main

import (
	"strings"
	"testing"
)

// TestClassifyRetry 覆盖 classifyRetry 的全部判定分支:
// 首次即通过/重试后通过/多轮失败原因一致(稳定不可达)/多轮失败原因不一致(不稳定)。
func TestClassifyRetry(t *testing.T) {
	cases := []struct {
		name     string
		valid    bool
		attempts int
		everTCP  bool
		reasons  []string
		wantStb  bool // 期望 unstable
		wantSub  string
	}{
		{
			name: "首次即通过-稳定有效", valid: true, attempts: 1,
			reasons: []string{"33.5ms 12B"}, wantStb: false, wantSub: "33.5ms 12B",
		},
		{
			name: "重试后通过-不稳定", valid: true, attempts: 2,
			everTCP: true, reasons: []string{"dial tcp 1.2.3.4:443: i/o timeout", "41.8ms 9B"},
			wantStb: true, wantSub: "重试1次后通过(不稳定)",
		},
		{
			name: "多轮失败-稳定不可达", valid: false, attempts: 3,
			reasons: []string{"dial tcp a:b: i/o timeout", "dial tcp a:b: i/o timeout", "dial tcp a:b: i/o timeout"},
			wantStb: false, wantSub: "稳定不可达",
		},
		{
			name: "多轮失败-原因不一致-不稳定", valid: false, attempts: 3,
			everTCP: true,
			reasons: []string{"dial tcp c:d: i/o timeout", "CONNECT拒绝: HTTP/1.1 400 Bad Request", "dial tcp c:d: i/o timeout"},
			wantStb: true, wantSub: "多次结果不一致",
		},
		{
			name: "多轮失败-原因不一致-期间曾TCP建连", valid: false, attempts: 2,
			everTCP: true,
			reasons: []string{"dial tcp e:f: i/o timeout", "CONNECT拒绝: HTTP/1.1 403 Forbidden"},
			wantStb: true, wantSub: "期间曾TCP建连",
		},
		{
			name: "不重试-单轮失败", valid: false, attempts: 1,
			reasons: []string{"dial tcp g:h: i/o timeout"},
			wantStb: false, wantSub: "共1次探测均失败[稳定不可达]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stb, detail := classifyRetry(tc.valid, tc.attempts, tc.everTCP, tc.reasons)
			if stb != tc.wantStb {
				t.Errorf("unstable = %v, 期望 %v (detail=%q)", stb, tc.wantStb, detail)
			}
			if !strings.Contains(detail, tc.wantSub) {
				t.Errorf("detail 未包含子串 %q: %q", tc.wantSub, detail)
			}
			// 无论何种判定, detail 都应保留最近一次探测的详情作为参考
			last := tc.reasons[len(tc.reasons)-1]
			if !strings.Contains(detail, last) {
				t.Errorf("detail 未保留最近一次连通性参考 %q: %q", last, detail)
			}
		})
	}
}
