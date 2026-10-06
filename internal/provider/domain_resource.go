package provider

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/krakenkey/terraform-provider-krakenkey/internal/client"
)

var (
	_ resource.Resource                = (*domainResource)(nil)
	_ resource.ResourceWithConfigure   = (*domainResource)(nil)
	_ resource.ResourceWithImportState = (*domainResource)(nil)
)

// ACME challenge zones. The API does not expose its zone, so the provider
// derives it from api_url; acme_zone overrides it.
const (
	defaultACMEZone = "acme.krakenkey.io"
	stagingACMEZone = "acme.dev.krakenkey.io"
	stagingAPIHost  = "api-dev.krakenkey.io"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// acmeZoneFor returns the zone the _acme-challenge CNAME points into: the
// override if set, the staging zone for the staging API, otherwise production.
func acmeZoneFor(apiURL, override string) string {
	if z := strings.Trim(strings.ToLower(strings.TrimSpace(override)), "."); z != "" {
		return z
	}
	if u, err := url.Parse(apiURL); err == nil && strings.EqualFold(u.Hostname(), stagingAPIHost) {
		return stagingACMEZone
	}
	return defaultACMEZone
}

func NewDomainResource() resource.Resource {
	return &domainResource{}
}

type domainResource struct {
	data *providerData
}

type domainModel struct {
	ID               types.String `tfsdk:"id"`
	Hostname         types.String `tfsdk:"hostname"`
	Verified         types.Bool   `tfsdk:"verified"`
	VerificationCode types.String `tfsdk:"verification_code"`
	TXTRecordName    types.String `tfsdk:"txt_record_name"`
	TXTRecordValue   types.String `tfsdk:"txt_record_value"`
	CNAMERecordName  types.String `tfsdk:"cname_record_name"`
	CNAMERecordValue types.String `tfsdk:"cname_record_value"`
	CreatedAt        types.String `tfsdk:"created_at"`
}

func (r *domainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}

func (r *domainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A domain registered in KrakenKey. Publish the TXT record, verify it with krakenkey_domain_verification, " +
			"and publish the _acme-challenge CNAME before requesting certificates for names under it. " +
			"Destroying it deletes the domain in KrakenKey for everyone in the organization. Certificates already issued " +
			"for its names stay valid and keep renewing, but new certificates for them are refused until the domain is " +
			"registered and verified again, which gives it a new verification code.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Domain UUID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"hostname": schema.StringAttribute{
				Description: "Domain name, for example example.com, in lowercase. A verified domain also covers its " +
					"subdomains and wildcards. Changing it forces a new resource.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:    []validator.String{domainHostnameValidator{}},
			},
			"verified": schema.BoolAttribute{
				Description: "Whether KrakenKey has verified ownership. KrakenKey re-checks the TXT record daily and clears this if it is gone.",
				Computed:    true,
			},
			"verification_code": computedString("Full TXT record value, krakenkey-site-verification=<hex>."),
			"txt_record_name":   computedString("Name of the TXT record to publish: the hostname itself."),
			"txt_record_value":  computedString("Value of the TXT record to publish. Same as verification_code."),
			"cname_record_name": computedString("Name of the CNAME record that delegates ACME challenges: _acme-challenge.<hostname>."),
			"cname_record_value": computedString("Target of the _acme-challenge CNAME: the hostname with dots replaced by dashes, " +
				"under the provider's acme_zone, for example example-com.acme.krakenkey.io."),
			"created_at": computedString("When the domain was registered, RFC 3339."),
		},
	}
}

func (r *domainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	r.data = data
}

func (r *domainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan domainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	c := r.data.client
	hostname := plan.Hostname.ValueString()

	// POST /domains returns the existing domain when the hostname is already
	// registered. List first so that case is an error rather than two
	// configurations silently sharing (and later deleting) one domain.
	existing, err := c.ListDomains(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not list domains", err.Error())
		return
	}
	known := map[string]bool{}
	for _, d := range existing {
		known[d.ID] = true
	}

	d, err := c.CreateDomain(ctx, hostname)
	if err != nil {
		resp.Diagnostics.AddError("Could not register "+hostname, err.Error())
		return
	}
	if known[d.ID] {
		resp.Diagnostics.AddError(hostname+" is already registered",
			fmt.Sprintf("KrakenKey already has this domain (ID %s). Import it instead: terraform import <address> %s", d.ID, d.ID))
		return
	}
	r.apply(d, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *domainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state domainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.data.client.GetDomain(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read domain "+state.ID.ValueString(), err.Error())
		return
	}
	r.apply(d, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is never called with a change, since hostname forces replacement.
func (r *domainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state domainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *domainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state domainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteDomain(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete domain "+state.ID.ValueString(), err.Error())
	}
}

func (r *domainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !uuidPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Invalid import ID", "Import a domain by its UUID.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// apply copies the API's view of the domain into m and derives the DNS records.
func (r *domainResource) apply(d *client.Domain, m *domainModel) {
	host := strings.ToLower(d.Hostname)
	m.ID = types.StringValue(d.ID)
	m.Hostname = types.StringValue(d.Hostname)
	m.Verified = types.BoolValue(d.IsVerified)
	m.VerificationCode = types.StringValue(d.VerificationCode)
	m.TXTRecordName = types.StringValue(d.Hostname)
	m.TXTRecordValue = types.StringValue(d.VerificationCode)
	m.CNAMERecordName = types.StringValue("_acme-challenge." + host)
	m.CNAMERecordValue = types.StringValue(strings.ReplaceAll(host, ".", "-") + "." + r.data.acmeZone)
	m.CreatedAt = types.StringValue(d.CreatedAt.UTC().Format(time.RFC3339))
}

// domainHostnameValidator catches the hostname forms the API stores as
// distinct domains or rejects with a less helpful message.
type domainHostnameValidator struct{}

func (domainHostnameValidator) Description(context.Context) string {
	return "a lowercase domain name without a wildcard or trailing dot"
}

func (v domainHostnameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (domainHostnameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	h := req.ConfigValue.ValueString()
	switch {
	case strings.Contains(h, "*"):
		resp.Diagnostics.AddAttributeError(req.Path, "Wildcard hostname",
			"Register the base domain (example.com, not *.example.com). A verified domain covers wildcard and subdomain names.")
	case strings.HasSuffix(h, "."):
		resp.Diagnostics.AddAttributeError(req.Path, "Trailing dot in hostname", "Remove the trailing dot.")
	case h != strings.ToLower(h):
		resp.Diagnostics.AddAttributeError(req.Path, "Hostname must be lowercase",
			"KrakenKey matches hostnames exactly, so "+h+" and "+strings.ToLower(h)+" would be separate domains. Use "+strings.ToLower(h)+".")
	}
}
