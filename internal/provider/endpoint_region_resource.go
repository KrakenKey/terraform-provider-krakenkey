package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/krakenkey/terraform-provider-krakenkey/internal/client"
)

var (
	_ resource.Resource                = (*endpointRegionResource)(nil)
	_ resource.ResourceWithConfigure   = (*endpointRegionResource)(nil)
	_ resource.ResourceWithImportState = (*endpointRegionResource)(nil)
)

func NewEndpointRegionResource() resource.Resource {
	return &endpointRegionResource{}
}

type endpointRegionResource struct {
	data *providerData
}

type endpointRegionModel struct {
	ID         types.String `tfsdk:"id"`
	EndpointID types.String `tfsdk:"endpoint_id"`
	Region     types.String `tfsdk:"region"`
}

func (r *endpointRegionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_region"
}

func (r *endpointRegionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Scans an endpoint from one KrakenKey hosted probe region. Needs the Starter plan or above.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "<endpoint_id>/<region>. Also the import ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"endpoint_id": schema.StringAttribute{
				Description:   "Endpoint UUID. Changing it creates a new region assignment.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"region": schema.StringAttribute{
				Description:   "Hosted probe region, for example us-east-1. Changing it creates a new region assignment.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *endpointRegionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *endpointRegionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan endpointRegionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	c := r.data.client
	endpointID, region := plan.EndpointID.ValueString(), plan.Region.ValueString()

	// Adding a region that is already on the endpoint succeeds without a
	// change, which would let two configurations share one region and remove
	// it from each other. Refuse and point at import instead.
	ep, err := c.GetEndpoint(ctx, endpointID)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not read endpoint %s", endpointID), err.Error())
		return
	}
	if ep.HasRegion(region) {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Region %s is already on endpoint %s", region, endpointID),
			fmt.Sprintf("Import it with `terraform import <address> %s/%s` instead of creating it.", endpointID, region),
		)
		return
	}
	if err := c.AddEndpointRegion(ctx, endpointID, region); err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not add region %s to endpoint %s", region, endpointID), err.Error())
		return
	}
	plan.ID = types.StringValue(endpointID + "/" + region)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *endpointRegionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state endpointRegionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	endpointID, region := state.EndpointID.ValueString(), state.Region.ValueString()
	ep, err := r.data.client.GetEndpoint(ctx, endpointID)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not read endpoint %s", endpointID), err.Error())
		return
	}
	if !ep.HasRegion(region) {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is never called: both arguments force replacement.
func (r *endpointRegionResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update is not supported", "krakenkey_endpoint_region has no arguments that can change in place.")
}

func (r *endpointRegionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state endpointRegionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	endpointID, region := state.EndpointID.ValueString(), state.Region.ValueString()
	err := r.data.client.RemoveEndpointRegion(ctx, endpointID, region)
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not remove region %s from endpoint %s", region, endpointID), err.Error())
	}
}

func (r *endpointRegionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Endpoint IDs are UUIDs, so the first slash ends the endpoint ID.
	endpointID, region, ok := strings.Cut(req.ID, "/")
	if !ok || endpointID == "" || region == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Use <endpoint_id>/<region>, for example 3f0c9a52-6b1e-4d8a-9c47-2e5f1b7d8a90/us-east-1.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("endpoint_id"), endpointID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("region"), region)...)
}
