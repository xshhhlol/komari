package accounts

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/utils"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
)

// constantSalt 是旧版 SHA256 方案的全局固定盐，仅保留用于校验/升级历史遗留的旧哈希，
// 新密码一律使用 argon2id（每用户随机盐）。不要再用它生成新哈希。
const constantSalt = "06Wm4Jv1Hkxx"

// argon2id 参数。登录频率低，可取较保守（更安全）的内存/迭代成本。
const (
	argonTime    = 2
	argonMemory  = 64 * 1024 // KiB = 64 MiB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// CheckPassword 检查密码是否正确
//
// 如果密码正确，返回用户的 UUID 和 true；否则返回空字符串和 false
func CheckPassword(username, passwd string) (uuid string, success bool) {
	db := dbcore.GetDBInstance()
	var user models.User
	result := db.Where("username = ?", username).First(&user)
	if result.Error != nil {
		// 静默处理错误，不显示日志
		return "", false
	}
	ok, legacy := verifyPassword(passwd, user.Passwd)
	if !ok {
		return "", false
	}
	// 透明升级：历史遗留的旧 SHA256 哈希在校验通过后立即重写为 argon2id。
	// 升级失败不阻断本次登录，仅记录日志，下次登录再尝试。
	if legacy {
		if err := db.Model(&models.User{}).Where("uuid = ?", user.UUID).
			Update("passwd", hashPasswd(passwd)).Error; err != nil {
			log.Printf("accounts: failed to upgrade password hash for user %s: %v", user.UUID, err)
		}
	}
	return user.UUID, true
}

// ForceResetPassword 强制重置用户密码
func ForceResetPassword(username, passwd string) (err error) {
	db := dbcore.GetDBInstance()
	result := db.Model(&models.User{}).Where("username = ?", username).Update("passwd", hashPasswd(passwd))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("无法找到用户名")
	}
	return nil
}

// hashPasswd 使用 argon2id（每用户随机盐）生成 PHC 编码的密码哈希。
// 输出形如 $argon2id$v=19$m=65536,t=2,p=4$<saltB64>$<hashB64>。
func hashPasswd(passwd string) string {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		// 读不到随机数时宁可 panic，也不要落一个弱哈希。
		panic("accounts: failed to read random salt: " + err.Error())
	}
	hash := argon2.IDKey([]byte(passwd), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

// hashPasswdLegacy 复刻旧版 SHA256+固定盐 方案，仅用于校验历史遗留哈希。
func hashPasswdLegacy(passwd string) string {
	hash := sha256.Sum256([]byte(passwd + constantSalt))
	return base64.StdEncoding.EncodeToString(hash[:])
}

// verifyPassword 校验明文密码是否匹配存储的哈希。
// 返回 (是否匹配, 是否为需要升级的旧格式哈希)。
func verifyPassword(passwd, stored string) (ok bool, legacy bool) {
	if strings.HasPrefix(stored, "$argon2id$") {
		return verifyArgon2id(passwd, stored), false
	}
	// 旧格式：SHA256(passwd + constantSalt) 的 base64。
	expected := hashPasswdLegacy(passwd)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(stored)) == 1, true
}

// verifyArgon2id 解析 PHC 编码并以恒定时间比较 argon2id 哈希。
func verifyArgon2id(passwd, encoded string) bool {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=65536,t=2,p=4", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(passwd), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func CreateAccount(username, passwd string) (user models.User, err error) {
	db := dbcore.GetDBInstance()
	hashedPassword := hashPasswd(passwd)
	user = models.User{
		UUID:     uuid.New().String(),
		Username: username,
		Passwd:   hashedPassword,
	}
	err = db.Create(&user).Error
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}

func DeleteAccountByUsername(username string) (err error) {
	db := dbcore.GetDBInstance()
	err = db.Where("username = ?", username).Delete(&models.User{}).Error
	if err != nil {
		return err
	}
	return nil
}

// 创建默认管理员账户，使用环境变量 ADMIN_USERNAME 作为用户名，环境变量 ADMIN_PASSWORD 作为密码
func CreateDefaultAdminAccount() (username, passwd string, err error) {
	db := dbcore.GetDBInstance()

	username = os.Getenv("ADMIN_USERNAME")
	if username == "" {
		username = "admin"
	}

	passwd = os.Getenv("ADMIN_PASSWORD")
	if passwd == "" {
		passwd = utils.GeneratePassword()
	}

	hashedPassword := hashPasswd(passwd)

	user := models.User{
		UUID:      uuid.New().String(),
		Username:  username,
		Passwd:    hashedPassword,
		SSOID:     "",
		CreatedAt: models.FromTime(time.Now()),
		UpdatedAt: models.FromTime(time.Now()),
	}

	err = db.Create(&user).Error
	if err != nil {
		return "", "", err
	}

	return username, passwd, nil
}

func GetUserByUUID(uuid string) (user models.User, err error) {
	db := dbcore.GetDBInstance()
	err = db.Where("uuid = ?", uuid).First(&user).Error
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}

// 通过 SSO 信息获取用户
func GetUserBySSO(ssoID string) (user models.User, err error) {
	db := dbcore.GetDBInstance()

	// 首先尝试查找已存在的用户
	err = db.Where("sso_id = ?", ssoID).First(&user).Error
	if err == nil {
		return user, nil
	}

	// 如果找不到用户，返回明确的错误信息
	return models.User{}, fmt.Errorf("用户不存在：%s", ssoID)
}

func BindingExternalAccount(uuid string, sso_id string) error {
	db := dbcore.GetDBInstance()
	err := db.Model(&models.User{}).Where("uuid = ?", uuid).Update("sso_id", sso_id).Error
	if err != nil {
		return err
	}
	return nil
}

func UnbindExternalAccount(uuid string) error {
	db := dbcore.GetDBInstance()
	err := db.Model(&models.User{}).Where("uuid = ?", uuid).Update("sso_id", "").Error
	if err != nil {
		return err
	}
	return nil
}

func UpdateUser(uuid string, name, password, sso_type *string) error {
	db := dbcore.GetDBInstance()
	// Check if user exists
	var existingUser models.User
	result := db.Where("uuid = ?", uuid).First(&existingUser)
	if result.Error != nil {
		return fmt.Errorf("user not found: %s", uuid)
	}
	updates := make(map[string]interface{})
	if name != nil {
		updates["username"] = *name
	}
	if password != nil {
		updates["passwd"] = hashPasswd(*password)
	}
	if sso_type != nil {
		updates["sso_type"] = *sso_type
	}
	updates["updated_at"] = time.Now()
	err := db.Model(&models.User{}).Where("uuid = ?", uuid).Updates(updates).Error
	if err != nil {
		return err
	}
	if password != nil {
		DeleteAllSessions()
	}
	return nil
}
