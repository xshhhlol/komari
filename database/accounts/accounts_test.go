package accounts

import (
	"strings"
	"testing"
)

func TestHashPasswdArgon2idRoundTrip(t *testing.T) {
	const pw = "correct horse battery staple"
	h := hashPasswd(pw)
	if !strings.HasPrefix(h, "$argon2id$") {
		t.Fatalf("expected argon2id PHC string, got %q", h)
	}
	// 同一密码两次哈希应不同（每用户随机盐）。
	if h2 := hashPasswd(pw); h == h2 {
		t.Fatal("two hashes of the same password are identical; salt is not random")
	}
	ok, legacy := verifyPassword(pw, h)
	if !ok || legacy {
		t.Fatalf("verifyPassword(correct) = (%v, legacy=%v), want (true, false)", ok, legacy)
	}
	if ok, _ := verifyPassword("wrong", h); ok {
		t.Fatal("verifyPassword accepted a wrong password")
	}
}

func TestVerifyPasswordLegacyUpgradePath(t *testing.T) {
	const pw = "legacy-password"
	legacyHash := hashPasswdLegacy(pw)

	// 旧哈希应能校验通过，并被标记为需要升级。
	ok, legacy := verifyPassword(pw, legacyHash)
	if !ok || !legacy {
		t.Fatalf("verifyPassword(legacy correct) = (%v, legacy=%v), want (true, true)", ok, legacy)
	}
	// 错误密码对旧哈希也应失败。
	if ok, _ := verifyPassword("nope", legacyHash); ok {
		t.Fatal("legacy verify accepted a wrong password")
	}
}

func TestVerifyArgon2idRejectsMalformed(t *testing.T) {
	bad := []string{
		"",
		"plaintext",
		"$argon2id$v=19$m=65536,t=2,p=4$onlyfourparts",
		"$argon2i$v=19$m=65536,t=2,p=4$c2FsdA$aGFzaA", // wrong variant
	}
	for _, s := range bad {
		if verifyArgon2id("x", s) {
			t.Fatalf("verifyArgon2id accepted malformed input %q", s)
		}
	}
}
