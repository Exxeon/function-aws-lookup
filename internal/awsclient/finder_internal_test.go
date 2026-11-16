package awsclient

import (
	"errors"
	"fmt"

	"github.com/aws/smithy-go"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = DescribeTable("isWrongType",
	func(err error, want bool) {
		Expect(isWrongType(err)).To(Equal(want))
	},
	Entry("resource not found", &smithy.GenericAPIError{Code: "ResourceNotFoundException"}, true),
	Entry("invalid identifier", fmt.Errorf("wrapped: %w", &smithy.GenericAPIError{Code: "InvalidRequestException"}), true),
	Entry("access denied", &smithy.GenericAPIError{Code: "AccessDeniedException"}, false),
	Entry("throttled", &smithy.GenericAPIError{Code: "ThrottlingException"}, false),
	Entry("not an API error", errors.New("connection reset"), false),
)
