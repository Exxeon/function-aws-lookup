package awsclient_test

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	rgttypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"

	"github.com/giepa/function-aws-lookup/internal/awsclient"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("StaticCredentials", func() {
	It("reads the default profile of a shared credentials file", func() {
		c, err := awsclient.StaticCredentials(map[string][]byte{
			"credentials": []byte("# comment\n[other]\naws_access_key_id = NOPE\naws_secret_access_key = nope\n\n[default]\naws_access_key_id = AKID\naws_secret_access_key = secret\naws_session_token = token\n"),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.AccessKeyID).To(Equal("AKID"))
		Expect(c.SecretAccessKey).To(Equal("secret"))
		Expect(c.SessionToken).To(Equal("token"))
	})

	It("uses the only profile when there is no default profile", func() {
		c, err := awsclient.StaticCredentials(map[string][]byte{
			"credentials": []byte("[profile adopt]\naws_access_key_id=AKID\naws_secret_access_key=secret\n"),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.AccessKeyID).To(Equal("AKID"))
	})

	It("reads separate keys", func() {
		c, err := awsclient.StaticCredentials(map[string][]byte{
			"aws_access_key_id":     []byte("AKID\n"),
			"aws_secret_access_key": []byte("secret"),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.AccessKeyID).To(Equal("AKID"))
		Expect(c.SecretAccessKey).To(Equal("secret"))
		Expect(c.SessionToken).To(BeEmpty())
	})

	It("returns an error when keys are missing", func() {
		_, err := awsclient.StaticCredentials(map[string][]byte{"aws_access_key_id": []byte("AKID")})
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("TagFilters", func() {
	It("converts tags to sorted filters", func() {
		Expect(awsclient.TagFilters(map[string][]string{"team": {"payments"}, "env": {"prod", "staging"}, "legacy": nil})).To(Equal([]rgttypes.TagFilter{
			{Key: aws.String("env"), Values: []string{"prod", "staging"}},
			{Key: aws.String("legacy")},
			{Key: aws.String("team"), Values: []string{"payments"}},
		}))
	})
})
