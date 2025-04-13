package bot

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"PakuchiBot/internal/repository"
	"PakuchiBot/internal/storage"
	"PakuchiBot/internal/utils"

	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/driver"
)

type BotConfig struct {
	Connection struct {
		WSAddress   string `mapstructure:"ws_address"`
		AccessToken string `mapstructure:"access_token"`
	} `mapstructure:"connection"`
	Bot struct {
		SelfID        int64    `mapstructure:"self_id"`
		CommandPrefix string   `mapstructure:"command_prefix"`
		Debug         bool     `mapstructure:"debug"`
		NickNames     []string `mapstructure:"nicknames"`
		SuperUsers    []int64  `mapstructure:"super_users"`
	} `mapstructure:"bot"`
	Storage struct {
		DBPath        string `mapstructure:"db_path"`
		EncryptionKey string `mapstructure:"encryption_key"`
	} `mapstructure:"storage"`
	Scheduler struct {
		CheckInterval int `mapstructure:"check_interval"`
		MaxRetries    int `mapstructure:"max_retries"`
	} `mapstructure:"scheduler"`
}

var (
	Config      BotConfig
	DB          *sqlx.DB
	UserRepo    *repository.UserRepository
	NotifyRepo  *repository.NotifyRepository
	TokenCrypto *utils.TokenCrypto
)

func generateRandomKey() string {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		timestamp := time.Now().UnixNano()
		for i := 0; i < 32; i++ {
			key[i] = byte((timestamp >> (i % 8)) & 0xFF)
		}
	}
	return base64.StdEncoding.EncodeToString(key)[:32]
}

func initDatabasePath(dbPath string) (string, error) {
	if strings.HasPrefix(dbPath, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		dbPath = filepath.Join(home, dbPath[1:])
	}

	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create database directory: %w", err)
	}

	return dbPath, nil
}

func initializeConfig() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := viper.Unmarshal(&Config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	if Config.Storage.EncryptionKey == "" || Config.Storage.EncryptionKey == "your-32-byte-encryption-key-here!!" {
		Config.Storage.EncryptionKey = generateRandomKey()
		logrus.Warnf("new encryption key generated")

		viper.Set("storage.encryption_key", Config.Storage.EncryptionKey)
		if err := viper.WriteConfig(); err != nil {
			logrus.Warnf("failed to write new key to config file: %v", err)
		}
	}

	dbPath, err := initDatabasePath(Config.Storage.DBPath)
	if err != nil {
		return fmt.Errorf("failed to initialize database path: %w", err)
	}
	Config.Storage.DBPath = dbPath

	return nil
}

func InitConfig() error {
	if err := initializeConfig(); err != nil {
		return err
	}

	var err error
	DB, err = sqlx.Open("sqlite", Config.Storage.DBPath)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	DB.SetMaxOpenConns(1)
	DB.SetMaxIdleConns(1)

	if err := DB.Ping(); err != nil {
		return fmt.Errorf("failed to test database connection: %w", err)
	}

	if err := storage.InitDB(Config.Storage.DBPath); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	UserRepo = repository.NewUserRepository(DB)

	NotifyRepo = repository.NewNotifyRepository(DB)

	TokenCrypto, err = utils.NewTokenCrypto(Config.Storage.EncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to initialize encryption tool: %w", err)
	}

	return nil
}

func InitBot() error {
	cfg := &zero.Config{
		NickName:      Config.Bot.NickNames,
		CommandPrefix: Config.Bot.CommandPrefix,
		SuperUsers:    Config.Bot.SuperUsers,
		Driver: []zero.Driver{
			driver.NewWebSocketClient(Config.Connection.WSAddress, Config.Connection.AccessToken),
		},
	}

	zero.Run(cfg)
	return nil
}
