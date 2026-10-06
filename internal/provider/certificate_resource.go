package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/krakenkey/terraform-provider-krakenkey/internal/client"
)

var (
	_ resource.Resource                = (*certificateResource)(nil)
	_ resource.ResourceWithConfigure   = (*certificateResource)(nil)
	_ resource.ResourceWithImportState = (*certificateResource)(nil)
)

const defaultCreateTimeout = 20 * time.Minute

func NewCertificateResource() resource.Resource {
	return &certificateResource{}
}

type certificateResource struct {
	data *providerData
}

type certificateModel struct {
	ID              types.String   `tfsdk:"id"`
	CSRPEM          types.String   `tfsdk:"csr_pem"`
	AutoRenew       types.Bool     `tfsdk:"auto_renew"`
	RevokeOnDestroy types.Bool     `tfsdk:"revoke_on_destroy"`
	Status          types.String   `tfsdk:"status"`
	FailureReason   types.String   `tfsdk:"failure_reason"`
	CertPEM         types.String   `tfsdk:"cert_pem"`
	ChainPEM        types.String   `tfsdk:"chain_pem"`
	FullchainPEM    types.String   `tfsdk:"fullchain_pem"`
	ExpiresAt       types.String   `tfsdk:"expires_at"`
	ExpiresAtUnix   types.Int64    `tfsdk:"expires_at_unix"`
	RenewalCount    types.Int64    `tfsdk:"renewal_count"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func (r *certificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (r *certificateResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A TLS certificate issued by KrakenKey from a CSR. KrakenKey never sees the private key. " +
			"Every name in the CSR must be on a verified domain in the account.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Certificate ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"csr_pem": schema.StringAttribute{
				Description:   "PEM certificate signing request, for example tls_cert_request.cert_request_pem. Changing it issues a new certificate.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"auto_renew": schema.BoolAttribute{
				Description: "Let KrakenKey renew the certificate inside the plan's renewal window, reusing the same CSR and key. Default true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"revoke_on_destroy": schema.BoolAttribute{
				Description: "Revoke the certificate when the resource is destroyed or replaced. Default false: the certificate " +
					"is only removed from state and stays valid in KrakenKey until it expires.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"status":         computedString("pending, issuing, issued, failed, renewing, revoking or revoked."),
			"failure_reason": computedString("Why the last issuance or renewal failed, if it did."),
			"cert_pem":       computedString("Leaf certificate. Most servers want fullchain_pem instead."),
			"chain_pem":      computedString("Intermediate certificates only."),
			"fullchain_pem":  computedString("Leaf plus intermediates. Deploy this one."),
			"expires_at":     computedString("Expiry, RFC 3339."),
			"expires_at_unix": schema.Int64Attribute{
				Description: "Expiry as a Unix timestamp. It changes on every renewal, so it works as a *_wo_version for " +
					"resources that need to re-read a write-only private key when the certificate changes.",
				Computed: true,
			},
			"renewal_count": schema.Int64Attribute{
				Description: "Number of renewals so far.",
				Computed:    true,
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{Create: true}),
		},
	}
}

func computedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{Description: desc, Computed: true}
}

func (r *certificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *certificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := plan.Timeouts.Create(ctx, defaultCreateTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c := r.data.client
	id, err := c.CreateCertificate(ctx, plan.CSRPEM.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not submit the CSR", err.Error())
		return
	}
	// Save the ID straight away so a timeout or failure below doesn't orphan the certificate.
	plan.ID = types.StringValue(strconv.Itoa(id))
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), plan.ID)...)

	if !plan.AutoRenew.ValueBool() {
		if err := c.SetAutoRenew(ctx, id, false); err != nil {
			resp.Diagnostics.AddError("Could not turn off auto-renew", err.Error())
			return
		}
	}

	cert, err := r.waitForIssued(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Certificate %d was not issued", id), err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, cert, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// waitForIssued polls until the certificate is issued, fails, or ctx ends.
func (r *certificateResource) waitForIssued(ctx context.Context, id int) (*client.Certificate, error) {
	ticker := time.NewTicker(r.data.pollInterval)
	defer ticker.Stop()
	for {
		cert, err := r.data.client.GetCertificate(ctx, id)
		if err != nil {
			return nil, err
		}
		switch cert.Status {
		case client.StatusIssued:
			return cert, nil
		case client.StatusFailed:
			reason := "no reason given"
			if cert.FailureReason != nil {
				reason = *cert.FailureReason
			}
			return nil, fmt.Errorf("issuance failed: %s", reason)
		case client.StatusPending, client.StatusIssuing:
		default:
			return nil, fmt.Errorf("unexpected status %q while waiting for issuance", cert.Status)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("still %s when the create timeout ran out; the certificate keeps going in KrakenKey and the next apply picks it up: %w", cert.Status, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (r *certificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state certificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid certificate ID", err.Error())
		return
	}
	cert, err := r.data.client.GetCertificate(ctx, id)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not read certificate %d", id), err.Error())
		return
	}
	// Imported resources have no CSR yet, so take it from the API. Otherwise keep
	// the configured value unless the content really differs, so surrounding
	// whitespace doesn't force a replacement.
	if state.CSRPEM.IsNull() || strings.TrimSpace(state.CSRPEM.ValueString()) != strings.TrimSpace(cert.RawCSR) {
		state.CSRPEM = types.StringValue(cert.RawCSR)
	}
	if state.RevokeOnDestroy.IsNull() {
		state.RevokeOnDestroy = types.BoolValue(false)
	}
	resp.Diagnostics.Append(r.apply(ctx, cert, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *certificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid certificate ID", err.Error())
		return
	}
	if !plan.AutoRenew.Equal(state.AutoRenew) {
		if err := r.data.client.SetAutoRenew(ctx, id, plan.AutoRenew.ValueBool()); err != nil {
			resp.Diagnostics.AddError("Could not update auto-renew", err.Error())
			return
		}
	}
	cert, err := r.data.client.GetCertificate(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not read certificate %d", id), err.Error())
		return
	}
	// Computed values carry over from state; apply refreshes them.
	plan.ID = state.ID
	copyComputed(&plan, &state)
	resp.Diagnostics.Append(r.apply(ctx, cert, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *certificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state certificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid certificate ID", err.Error())
		return
	}
	if !state.RevokeOnDestroy.ValueBool() {
		resp.Diagnostics.AddWarning(
			fmt.Sprintf("Certificate %d kept in KrakenKey", id),
			"The certificate was removed from Terraform state but is still valid, and KrakenKey keeps renewing it if auto-renew is on. "+
				"Revoke it in the dashboard or with `krakenkey cert revoke "+state.ID.ValueString()+"` if it is no longer used, "+
				"or set revoke_on_destroy = true.",
		)
		return
	}
	if err := r.data.client.RevokeCertificate(ctx, id); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not revoke certificate %d", id), err.Error())
	}
}

func (r *certificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, err := strconv.Atoi(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "Import a certificate by its numeric ID.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// apply copies the API's view of the certificate into m. The PEMs only change
// while the certificate is issued: during a renewal or after a failure the
// last issued certificate is kept, so resources that deploy it don't see it
// disappear.
func (r *certificateResource) apply(ctx context.Context, cert *client.Certificate, m *certificateModel) (diags diag.Diagnostics) {
	m.Status = types.StringValue(cert.Status)
	m.AutoRenew = types.BoolValue(cert.AutoRenew)
	m.RenewalCount = types.Int64Value(int64(cert.RenewalCount))
	if cert.FailureReason != nil {
		m.FailureReason = types.StringValue(*cert.FailureReason)
	} else {
		m.FailureReason = types.StringNull()
	}

	if cert.Status != client.StatusIssued || cert.CrtPEM == nil {
		nullIfUnknown(m)
		return nil
	}
	chain, err := r.data.client.GetChain(ctx, cert.ID)
	if err != nil {
		diags.AddError(fmt.Sprintf("Could not read the chain for certificate %d", cert.ID), err.Error())
		return diags
	}
	m.CertPEM = types.StringValue(*cert.CrtPEM)
	m.ChainPEM = types.StringValue(chain.ChainPEM)
	m.FullchainPEM = types.StringValue(chain.FullChainPEM)
	if cert.ExpiresAt != nil {
		m.ExpiresAt = types.StringValue(cert.ExpiresAt.UTC().Format(time.RFC3339))
		m.ExpiresAtUnix = types.Int64Value(cert.ExpiresAt.Unix())
	}
	return nil
}

func copyComputed(dst, src *certificateModel) {
	dst.CertPEM, dst.ChainPEM, dst.FullchainPEM = src.CertPEM, src.ChainPEM, src.FullchainPEM
	dst.ExpiresAt, dst.ExpiresAtUnix = src.ExpiresAt, src.ExpiresAtUnix
}

// nullIfUnknown turns still-unknown computed values into nulls before state is saved.
func nullIfUnknown(m *certificateModel) {
	for _, s := range []*types.String{&m.CertPEM, &m.ChainPEM, &m.FullchainPEM, &m.ExpiresAt} {
		if s.IsUnknown() {
			*s = types.StringNull()
		}
	}
	if m.ExpiresAtUnix.IsUnknown() {
		m.ExpiresAtUnix = types.Int64Null()
	}
}
