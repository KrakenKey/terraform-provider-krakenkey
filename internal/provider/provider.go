package provider

import (
	"context"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/krakenkey/terraform-provider-krakenkey/internal/client"
)

var _ provider.Provider = (*krakenkeyProvider)(nil)

type krakenkeyProvider struct {
	version string
	// pollInterval is how often create waits re-check a certificate. Tests shorten it.
	pollInterval time.Duration
}

// providerData is handed to every resource through Configure.
type providerData struct {
	client       *client.Client
	pollInterval time.Duration
}

type providerModel struct {
	APIKey types.String `tfsdk:"api_key"`
	APIURL types.String `tfsdk:"api_url"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &krakenkeyProvider{version: version, pollInterval: 10 * time.Second}
	}
}

func (p *krakenkeyProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "krakenkey"
	resp.Version = p.version
}

func (p *krakenkeyProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage TLS certificates, domains and endpoint monitoring in KrakenKey.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Description: "KrakenKey API key (kk_...). Defaults to the KK_API_KEY environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"api_url": schema.StringAttribute{
				Description: "KrakenKey API base URL. Defaults to KK_API_URL, then " + client.DefaultAPIURL + ".",
				Optional:    true,
			},
		},
	}
}

func (p *krakenkeyProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := os.Getenv("KK_API_KEY")
	if !cfg.APIKey.IsNull() && !cfg.APIKey.IsUnknown() {
		apiKey = cfg.APIKey.ValueString()
	}
	apiURL := os.Getenv("KK_API_URL")
	if !cfg.APIURL.IsNull() && !cfg.APIURL.IsUnknown() {
		apiURL = cfg.APIURL.ValueString()
	}
	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(path.Root("api_key"), "Missing KrakenKey API key",
			"Set api_key in the provider block or the KK_API_KEY environment variable.")
		return
	}

	data := &providerData{
		client:       client.New(apiURL, apiKey, "terraform-provider-krakenkey/"+p.version),
		pollInterval: p.pollInterval,
	}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *krakenkeyProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewCertificateResource,
		NewEndpointResource,
		NewEndpointRegionResource,
	}
}

func (p *krakenkeyProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewCertificateDataSource,
		NewEndpointDataSource,
	}
}
