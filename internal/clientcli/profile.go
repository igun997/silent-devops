package clientcli

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Profile struct {
	Address    string `json:"address"`
	ServerName string `json:"server_name"`
	CAPath     string `json:"ca_path"`
	Username   string `json:"username"`
}
type Invitation struct {
	Address string `json:"address"`
	Pin     string `json:"pin"`
	Secret  string `json:"secret"`
}

func DecodeInvitation(code string) (Invitation, error) {
	if !strings.HasPrefix(code, "SDC1_") {
		return Invitation{}, errors.New("invalid client invitation")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(code, "SDC1_"))
	if err != nil {
		return Invitation{}, errors.New("invalid client invitation")
	}
	var v Invitation
	if json.Unmarshal(raw, &v) != nil || v.Address == "" || v.Pin == "" || v.Secret == "" {
		return Invitation{}, errors.New("invalid client invitation")
	}
	return v, nil
}
func SaveProfile(path string, p Profile) error {
	if p.Address == "" || p.CAPath == "" {
		return errors.New("invalid profile")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
func LoadProfile(path string) (Profile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Profile{}, err
	}
	if info.Mode().Perm()&0077 != 0 {
		return Profile{}, errors.New("profile permissions too broad")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	err = json.Unmarshal(data, &p)
	return p, err
}
