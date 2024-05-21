package provider

import (
	"context"
	"net/url"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/helpers/validatordiag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ validator.String = urlValidator{}

type urlValidator struct {
	description string
}

func (v urlValidator) Description(_ context.Context) string {
	return v.description
}

func (v urlValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v urlValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue

	invalid := false
	if value.Equal(types.StringValue("")) {
		invalid = true
	} else {
		_, err := url.ParseRequestURI(value.ValueString())
		if err != nil {
			invalid = true
		}
	}

	if invalid {
		response.Diagnostics.Append(validatordiag.InvalidAttributeValueMatchDiagnostic(
			request.Path,
			v.Description(ctx),
			value.String(),
		))
	}
}

func URLValidator(description string) validator.String {
	return urlValidator{description}
}

var _ validator.String = diskSizeValidator{}

type diskSizeValidator struct {
	description string
}

func (v diskSizeValidator) Description(_ context.Context) string {
	return v.description
}

func (v diskSizeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v diskSizeValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	val := request.ConfigValue

	invalid := false
	if val.Equal(types.StringValue("")) {
		invalid = true
	} else {
		re := regexp.MustCompile(`^\d+[MG]?$`)
		invalid = !re.MatchString(val.ValueString())
	}

	if invalid {
		response.Diagnostics.Append(validatordiag.InvalidAttributeValueMatchDiagnostic(
			request.Path,
			v.Description(ctx),
			val.String(),
		))
	}
}

func DiskSizeValidator(description string) validator.String {
	return diskSizeValidator{description}
}
