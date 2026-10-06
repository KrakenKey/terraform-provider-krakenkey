package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/krakenkey/terraform-provider-krakenkey/internal/client"
)

var (
	_ resource.Resource                = (*endpointResource)(nil)
	_ resource.ResourceWithConfigure   = (*endpointResource)(nil)
	_ resource.ResourceWithImportState = (*endpointResource)(nil)
)

func NewEndpointResource() resource.Resource {
	return &endpointResource{}
}

type endpointResource struct {
	data *providerData
}

type endpointModel struct {
	ID        types.String `tfsdk:"id"`
	Host      types.String `tfsdk:"host"`
	Port      types.Int64  `tfsdk:"port"`
	SNI       types.String `tfsdk:"sni"`
	Label     types.String `tfsdk:"label"`
	IsActive  types.Bool   `tfsdk:"is_active"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (r *endpointResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint"
}

func (r *endpointResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A TLS endpoint that KrakenKey monitors. Hosted probe regions are managed with krakenkey_endpoint_region.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Endpoint UUID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"host": schema.StringAttribute{
				Description:   "Hostname to scan. Changing it creates a new endpoint.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"port": schema.Int64Attribute{
				Description: "Port to scan. Default 443. Changing it creates a new endpoint.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(443),
				Validators:  []validator.Int64{endpointPortValidator{}},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"sni": schema.StringAttribute{
				Description:   "SNI override. KrakenKey uses host when it is not set.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"label": schema.StringAttribute{
				Description: "Display label, up to 100 characters.",
				Optional:    true,
			},
			"is_active": schema.BoolAttribute{
				Description: "Whether scanning is on. Default true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"created_at": schema.StringAttribute{
				Description:   "Creation time, RFC 3339.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *endpointResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *endpointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan endpointModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	c := r.data.client
	host, port := plan.Host.ValueString(), int(plan.Port.ValueInt64())

	// POST /endpoints returns an existing endpoint for the same host and port
	// and overwrites its label and sni. Look first, so Terraform never takes
	// over (and later deletes) an endpoint it did not create.
	existing, err := c.ListEndpoints(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not list endpoints", err.Error())
		return
	}
	for _, e := range existing {
		if e.Host == host && e.Port == port {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Endpoint %s:%d already exists", host, port),
				fmt.Sprintf("KrakenKey already monitors %s:%d (endpoint %s). Import it with `terraform import <address> %s` "+
					"instead of creating it, or delete it in KrakenKey first.", host, port, e.ID, e.ID),
			)
			return
		}
	}

	ep, err := c.CreateEndpoint(ctx, host, port, optionalString(plan.SNI), optionalString(plan.Label))
	if err != nil {
		resp.Diagnostics.AddError("Could not create the endpoint", err.Error())
		return
	}
	// Save the ID straight away so a failure below doesn't orphan the endpoint.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), ep.ID)...)

	// The create call has no is_active field, so turning scanning off is a second call.
	if !plan.IsActive.ValueBool() && ep.IsActive {
		ep, err = c.UpdateEndpoint(ctx, ep.ID, map[string]any{"isActive": false})
		if err != nil {
			resp.Diagnostics.AddError("Could not turn off scanning", err.Error())
			return
		}
	}
	applyEndpoint(ep, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *endpointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state endpointModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ep, err := r.data.client.GetEndpoint(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not read endpoint %s", state.ID.ValueString()), err.Error())
		return
	}
	applyEndpoint(ep, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *endpointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state endpointModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Only host and port are immutable, and they force replacement, so the
	// remaining differences can all go in one PATCH. A null label clears it.
	fields := map[string]any{}
	if !plan.SNI.Equal(state.SNI) && !plan.SNI.IsUnknown() {
		fields["sni"] = optionalString(plan.SNI)
	}
	if !plan.Label.Equal(state.Label) {
		fields["label"] = optionalString(plan.Label)
	}
	if !plan.IsActive.Equal(state.IsActive) {
		fields["isActive"] = plan.IsActive.ValueBool()
	}
	plan.ID = state.ID
	if len(fields) > 0 {
		ep, err := r.data.client.UpdateEndpoint(ctx, state.ID.ValueString(), fields)
		if err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Could not update endpoint %s", state.ID.ValueString()), err.Error())
			return
		}
		applyEndpoint(ep, &plan)
	} else {
		plan.SNI, plan.CreatedAt = state.SNI, state.CreatedAt
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *endpointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state endpointModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.data.client.DeleteEndpoint(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not delete endpoint %s", state.ID.ValueString()), err.Error())
	}
}

func (r *endpointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyEndpoint copies the API's view of the endpoint into m.
func applyEndpoint(ep *client.Endpoint, m *endpointModel) {
	m.ID = types.StringValue(ep.ID)
	m.Host = types.StringValue(ep.Host)
	m.Port = types.Int64Value(int64(ep.Port))
	m.SNI = nullableString(ep.SNI)
	m.Label = nullableString(ep.Label)
	m.IsActive = types.BoolValue(ep.IsActive)
	m.CreatedAt = types.StringValue(ep.CreatedAt.UTC().Format(time.RFC3339))
}

// optionalString returns nil for a null or unknown value.
func optionalString(s types.String) *string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	v := s.ValueString()
	return &v
}

func nullableString(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

// endpointPortValidator keeps port in the range the API accepts.
type endpointPortValidator struct{}

func (endpointPortValidator) Description(context.Context) string {
	return "must be between 1 and 65535"
}

func (v endpointPortValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (endpointPortValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if p := req.ConfigValue.ValueInt64(); p < 1 || p > 65535 {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid port", fmt.Sprintf("port is %d; it must be between 1 and 65535.", p))
	}
}
