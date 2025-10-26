package api

// Type aliases to expose common types in the api package
import api_types "github.com/jack-barr3tt/gbr-engine/src/common/api-types"

type (
	ErrorResponse       = api_types.ErrorResponse
	HealthResponse      = api_types.HealthResponse
	Location            = api_types.Location
	NotFoundResponse    = api_types.NotFoundResponse
	Operator            = api_types.Operator
	ScheduleLocation    = api_types.ScheduleLocation
	ServiceResponse     = api_types.ServiceResponse
	ServiceQueryRequest = api_types.ServiceQueryRequest
	LocationFilter      = api_types.LocationFilter
)
