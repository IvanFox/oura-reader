"""Mirror of internal/oura/endpoints.go — the subset of fields the MCP uses.

Parity with the Go Registry is enforced by `make check-endpoints`. Only these
four fields matter to the MCP surface:
  - name: the endpoint identifier (used in URLs and tool names)
  - has_dates: whether the tool exposes start_date/end_date params
  - is_list: whether the tool exposes a limit param (vs. single-object)
  - has_day_field: whether the Go side indexes by day (informational here;
    surfaces via the parity check so drift fails fast)
"""

from dataclasses import dataclass


@dataclass(frozen=True)
class EndpointSpec:
    name: str
    has_dates: bool
    is_list: bool
    has_day_field: bool


REGISTRY: tuple[EndpointSpec, ...] = (
    EndpointSpec("daily_sleep",              has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("sleep",                    has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("sleep_time",               has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("daily_activity",           has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("daily_readiness",          has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("heartrate",                has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("daily_resilience",         has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("daily_stress",             has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("daily_spo2",               has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("daily_cardiovascular_age", has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("vo2_max",                  has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("workout",                  has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("session",                  has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("tag",                      has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("enhanced_tag",             has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("ring_configuration",       has_dates=False, is_list=True,  has_day_field=False),
    EndpointSpec("rest_mode_period",         has_dates=True,  is_list=True,  has_day_field=True),
    EndpointSpec("personal_info",            has_dates=False, is_list=False, has_day_field=False),
)

REGISTRY_BY_NAME: dict[str, EndpointSpec] = {spec.name: spec for spec in REGISTRY}
ENDPOINT_NAMES: tuple[str, ...] = tuple(spec.name for spec in REGISTRY)
