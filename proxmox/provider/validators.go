package provider

import (
	"context"
	"net/url"
	"regexp"
	"strconv"

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
	description   string
	requireSuffix bool
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
		var re *regexp.Regexp
		if v.requireSuffix {
			re = regexp.MustCompile(`^\d+[MG]$`)
		} else {
			re = regexp.MustCompile(`^\d+[MG]?$`)
		}
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

func DiskSizeValidator(description string, requireSuffix bool) validator.String {
	return diskSizeValidator{description, requireSuffix}
}

var _ validator.String = ipValidator{}

type ipValidator struct {
	description string
}

func (v ipValidator) Description(_ context.Context) string {
	return v.description
}

func (v ipValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v ipValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	val := request.ConfigValue

	invalid := false
	if val.Equal(types.StringValue("")) {
		invalid = true
	} else {
		re := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)\.(\d+)$`)
		m := re.FindStringSubmatch(val.ValueString())
		if m == nil {
			invalid = true
		} else {
			if val, err := strconv.Atoi(m[1]); err != nil || val < 0 || val > 255 {
				invalid = true
			}
			if val, err := strconv.Atoi(m[2]); err != nil || val < 0 || val > 255 {
				invalid = true
			}
			if val, err := strconv.Atoi(m[3]); err != nil || val < 0 || val > 255 {
				invalid = true
			}
			if val, err := strconv.Atoi(m[4]); err != nil || val < 0 || val > 255 {
				invalid = true
			}
		}
	}

	if invalid {
		response.Diagnostics.Append(validatordiag.InvalidAttributeValueMatchDiagnostic(
			request.Path,
			v.Description(ctx),
			val.String(),
		))
	}
}

func IPValidator(description string) validator.String {
	return ipValidator{description}
}

var _ validator.String = ipCidrValidator{}

type ipCidrValidator struct {
	description string
}

func (v ipCidrValidator) Description(_ context.Context) string {
	return v.description
}

func (v ipCidrValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v ipCidrValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	val := request.ConfigValue

	invalid := false
	if val.Equal(types.StringValue("")) {
		invalid = true
	} else {
		re := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)\.(\d+)/(\d+)$`)
		m := re.FindStringSubmatch(val.ValueString())
		if m == nil {
			invalid = true
		} else {
			if val, err := strconv.Atoi(m[1]); err != nil || val < 0 || val > 255 {
				invalid = true
			}
			if val, err := strconv.Atoi(m[2]); err != nil || val < 0 || val > 255 {
				invalid = true
			}
			if val, err := strconv.Atoi(m[3]); err != nil || val < 0 || val > 255 {
				invalid = true
			}
			if val, err := strconv.Atoi(m[4]); err != nil || val < 0 || val > 255 {
				invalid = true
			}
			if val, err := strconv.Atoi(m[5]); err != nil || val < 0 || val > 32 {
				invalid = true
			}
		}
	}

	if invalid {
		response.Diagnostics.Append(validatordiag.InvalidAttributeValueMatchDiagnostic(
			request.Path,
			v.Description(ctx),
			val.String(),
		))
	}
}

func IPCidrValidator(description string) validator.String {
	return ipCidrValidator{description}
}

var _ validator.String = sdnZoneValidator{}

type sdnZoneValidator struct {
	description string
}

func (v sdnZoneValidator) Description(_ context.Context) string {
	return v.description
}

func (v sdnZoneValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v sdnZoneValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	val := request.ConfigValue

	invalid := false
	if val.Equal(types.StringValue("")) {
		invalid = true
	} else {
		re := regexp.MustCompile(`^[a-zA-Z0-9]{2,8}$`)
		m := re.FindStringSubmatch(val.ValueString())
		if m == nil {
			invalid = true
		}
	}

	if invalid {
		response.Diagnostics.Append(validatordiag.InvalidAttributeValueMatchDiagnostic(
			request.Path,
			v.Description(ctx),
			val.String(),
		))
	}
}

func SDNZoneValidator(description string) validator.String {
	return sdnZoneValidator{description}
}
