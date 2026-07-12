package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxBearerTokenFileSize = 4 << 10

func resolveBearerTokenFiles(cfg *Config) error {
	if err := resolveBearerTokenFile(
		"auth",
		&cfg.Auth.BearerToken,
		&cfg.Auth.BearerTokenFile,
	); err != nil {
		return err
	}
	return resolveBearerTokenFile(
		"control_plane",
		&cfg.ControlPlane.BearerToken,
		&cfg.ControlPlane.BearerTokenFile,
	)
}

func resolveBearerTokenFile(section string, token, tokenFile *string) error {
	path := strings.TrimSpace(os.ExpandEnv(*tokenFile))
	*tokenFile = path
	if path == "" {
		return nil
	}
	if strings.TrimSpace(*token) != "" {
		if !isUnsafeToken(*token) {
			return fmt.Errorf("%s.bearer_token and %s.bearer_token_file are mutually exclusive", section, section)
		}
		*token = ""
	}
	resolved, err := readBearerTokenFile(path)
	if err != nil {
		return fmt.Errorf("read %s.bearer_token_file: %w", section, err)
	}
	*token = resolved
	return nil
}

func readBearerTokenFile(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("bearer token file path must be absolute")
	}
	if err := validateSecretDirectory(filepath.Dir(path)); err != nil {
		return "", err
	}

	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect bearer token file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("bearer token file must be a regular file, not a symlink")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("bearer token file permissions %04o are too broad", info.Mode().Perm())
	}
	if info.Size() > maxBearerTokenFileSize {
		return "", fmt.Errorf("bearer token file exceeds %d bytes", maxBearerTokenFileSize)
	}

	file, err := openSecretFileNoFollow(path)
	if err != nil {
		return "", fmt.Errorf("open bearer token file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()
	openedInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("inspect opened bearer token file: %w", err)
	}
	if !os.SameFile(info, openedInfo) {
		return "", errors.New("bearer token file changed while it was being opened")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBearerTokenFileSize+1))
	if err != nil {
		return "", fmt.Errorf("read bearer token file: %w", err)
	}
	if len(data) > maxBearerTokenFileSize {
		return "", fmt.Errorf("bearer token file exceeds %d bytes", maxBearerTokenFileSize)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", errors.New("bearer token file is empty")
	}
	return token, nil
}

func validateSecretDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect bearer token directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("bearer token directory must be a directory, not a symlink")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("bearer token directory permissions %04o are too broad", info.Mode().Perm())
	}
	return nil
}
