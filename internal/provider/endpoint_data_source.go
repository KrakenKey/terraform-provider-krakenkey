package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = (*endpointDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*endpointDataSource)(nil)
)

func NewEndpointDataSource() datasource.DataSource {
	return &endpointDataSource{}
}

type endpointDataSource struct {
	data *providerData
}

type endpointDataModel struct {
	ID            types.String `tfsdk:"id"`
	Host          types.String `tfsdk:"host"`
	Port          types.Int64  `tfsdk:"port"`
	SNI           types.String `tfsdk:"sni"`
	Label         types.String `tfsdk:"label"`
	IsActive      types.Bool   `tfsdk:"is_active"`
	CreatedAt     types.String `tfsdk:"created_at"`
	HostedRegions types.List   `tfsdk:"hosted_regions"`
}

func (d *endpointDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint"
}

func (d *endpointDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a monitored endpoint by ID.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Description: "Endpoint UUID.", Required: true},
			"host":       schema.StringAttribute{Description: "Hostname being scanned.", Computed: true},
			"port":       schema.Int64Attribute{Description: "Port being scanned.", Computed: true},
			"sni":        schema.StringAttribute{Description: "SNI override, if one is set.", Computed: true},
			"label":      schema.StringAttribute{Description: "Display label.", Computed: true},
			"is_active":  schema.BoolAttribute{Description: "Whether scanning is on.", Computed: true},
			"created_at": schema.StringAttribute{Description: "Creation time, RFC 3339.", Computed: true},
			"hosted_regions": schema.ListAttribute{
				Description: "Hosted probe regions that scan this endpoint. Manage them with krakenkey_endpoint_region.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *endpointDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *endpointDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg endpointDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ep, err := d.data.client.GetEndpoint(ctx, cfg.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not read endpoint %s", cfg.ID.ValueString()), err.Error())
		return
	}
	// The resource model has the same scalar fields; reuse its mapping.
	m := endpointModel{}
	applyEndpoint(ep, &m)
	regions, diags := types.ListValueFrom(ctx, types.StringType, ep.RegionNames())
	resp.Diagnostics.Append(diags...)
	cfg = endpointDataModel{
		ID: m.ID, Host: m.Host, Port: m.Port, SNI: m.SNI, Label: m.Label,
		IsActive: m.IsActive, CreatedAt: m.CreatedAt, HostedRegions: regions,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
