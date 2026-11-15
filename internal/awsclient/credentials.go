package awsclient

import (
	"bufio"
	"bytes"
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// Keys in the credentials secret.
const (
	KeyCredentials     = "credentials"
	KeyAccessKeyID     = "aws_access_key_id"
	KeySecretAccessKey = "aws_secret_access_key"
	KeySessionToken    = "aws_session_token"
)

const defaultProfile = "default"

// StaticCredentials reads AWS credentials from secret data. The data either
// has a credentials key with an AWS shared credentials file, the format that
// provider-aws uses, or separate aws_access_key_id, aws_secret_access_key and
// optional aws_session_token keys.
func StaticCredentials(data map[string][]byte) (aws.Credentials, error) {
	values := map[string]string{}
	if f, ok := data[KeyCredentials]; ok {
		values = parseProfile(f, defaultProfile)
	} else {
		for k, v := range data {
			values[k] = strings.TrimSpace(string(v))
		}
	}

	c := aws.Credentials{
		AccessKeyID:     values[KeyAccessKeyID],
		SecretAccessKey: values[KeySecretAccessKey],
		SessionToken:    values[KeySessionToken],
		Source:          "function-aws-lookup",
	}
	if c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return aws.Credentials{}, errors.New("credentials must contain aws_access_key_id and aws_secret_access_key")
	}
	return c, nil
}

// parseProfile returns the keys of a profile in a shared credentials file. If
// the file has no such profile but has exactly one profile, that one is used.
func parseProfile(file []byte, profile string) map[string]string {
	profiles := map[string]map[string]string{}
	current := ""

	s := bufio.NewScanner(bytes.NewReader(file))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		switch {
		case line == "", strings.HasPrefix(line, "#"), strings.HasPrefix(line, ";"):
			continue
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			current = strings.TrimSpace(strings.TrimPrefix(strings.Trim(line, "[]"), "profile "))
			if profiles[current] == nil {
				profiles[current] = map[string]string{}
			}
		default:
			k, v, ok := strings.Cut(line, "=")
			if !ok || profiles[current] == nil {
				continue
			}
			profiles[current][strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}

	if p, ok := profiles[profile]; ok {
		return p
	}
	if len(profiles) == 1 {
		for _, p := range profiles {
			return p
		}
	}
	return map[string]string{}
}
