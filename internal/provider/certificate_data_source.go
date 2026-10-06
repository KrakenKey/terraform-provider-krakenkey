package provider

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/krakenkey/terraform-provider-krakenkey/internal/client"
)

var (
	_ datasource.DataSource              = (*certificateDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*certificateDataSource)(nil)
)

func NewCertificateDataSource() datasource.DataSource {
	return &certificateDataSource{}
}

type certificateDataSource struct {
	data *providerData
}

type certificateDataModel struct {
	ID            types.String `tfsdk:"id"`
	CSRPEM        types.String `tfsdk:"csr_pem"`
	AutoRenew     types.Bool   `tfsdk:"auto_renew"`
	Status        types.String `tfsdk:"status"`
	FailureReason types.String `tfsdk:"failure_reason"`
	CertPEM       types.String `tfsdk:"cert_pem"`
	ChainPEM      types.String `tfsdk:"chain_pem"`
	FullchainPEM  types.String `tfsdk:"fullchain_pem"`
	ExpiresAt     types.String `tfsdk:"expires_at"`
	ExpiresAtUnix types.Int64  `tfsdk:"expires_at_unix"`
	RenewalCount  types.Int64  `tfsdk:"renewal_count"`
}

func (d *certificateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (d *certificateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	str := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Description: desc, Computed: true}
	}
	resp.Schema = schema.Schema{
		Description: "Reads an existing KrakenKey TLS certificate by ID. The PEM attributes and expiry are only set while the certificate is issued.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Description: "Certificate ID.", Required: true},
			"csr_pem":        str("PEM certificate signing request the certificate was issued from."),
			"auto_renew":     schema.BoolAttribute{Description: "Whether KrakenKey renews the certificate inside the plan's renewal window.", Computed: true},
			"status":         str("pending, issuing, issued, failed, renewing, revoking or revoked."),
			"failure_reason": str("Why the last issuance or renewal failed, if it did."),
			"cert_pem":       str("Leaf certificate. Most servers want fullchain_pem instead."),
			"chain_pem":      str("Intermediate certificates only."),
			"fullchain_pem":  str("Leaf plus intermediates. Deploy this one."),
			"expires_at":     str("Expiry, RFC 3339."),
			"expires_at_unix": schema.Int64Attribute{
				Description: "Expiry as a Unix timestamp. It changes on every renewal.",
				Computed:    true,
			},
			"renewal_count": schema.Int64Attribute{
				Description: "Number of renewals so far.",
				Computed:    true,
			},
		},
	}
}

func (d *certificateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	d.data = data
}

func (d *certificateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m certificateDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.Atoi(m.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid certificate ID", "A certificate ID is a number, for example \"42\".")
		return
	}
	cert, err := d.data.client.GetCertificate(ctx, id)
	if client.IsNotFound(err) {
		resp.Diagnostics.AddError(fmt.Sprintf("Certificate %d not found", id),
			"No certificate with this ID exists in the account the API key belongs to.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not read certificate %d", id), err.Error())
		return
	}

	m.CSRPEM = types.StringValue(cert.RawCSR)
	m.AutoRenew = types.BoolValue(cert.AutoRenew)
	m.Status = types.StringValue(cert.Status)
	m.RenewalCount = types.Int64Value(int64(cert.RenewalCount))
	m.FailureReason = types.StringNull()
	if cert.FailureReason != nil {
		m.FailureReason = types.StringValue(*cert.FailureReason)
	}
	m.CertPEM, m.ChainPEM, m.FullchainPEM = types.StringNull(), types.StringNull(), types.StringNull()
	m.ExpiresAt, m.ExpiresAtUnix = types.StringNull(), types.Int64Null()

	issued, err := fetchIssued(ctx, d.data.client, cert)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not read the chain for certificate %d", id), err.Error())
		return
	}
	if issued != nil {
		m.CertPEM = types.StringValue(issued.certPEM)
		m.ChainPEM = types.StringValue(issued.chainPEM)
		m.FullchainPEM = types.StringValue(issued.fullchainPEM)
		if issued.expiresAt != nil {
			m.ExpiresAt = types.StringValue(issued.expiresAt.UTC().Format(time.RFC3339))
			m.ExpiresAtUnix = types.Int64Value(issued.expiresAt.Unix())
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
