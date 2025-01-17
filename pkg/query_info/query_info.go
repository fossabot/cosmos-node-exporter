package query_info

import (
	"main/pkg/constants"
	"main/pkg/utils"

	"github.com/prometheus/client_golang/prometheus"
)

type QueryInfo struct {
	Module  string
	Action  string
	Success bool
}

func GetQueryInfoMetrics(allQueries map[string][]QueryInfo) []prometheus.Collector {
	querySuccessfulGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: constants.MetricsPrefix + "query_successful",
			Help: "Was query successful?",
		},
		[]string{"querier", "module", "action"},
	)

	for name, queryInfos := range allQueries {
		for _, queryInfo := range queryInfos {
			querySuccessfulGauge.
				With(prometheus.Labels{
					"querier": name,
					"module":  queryInfo.Module,
					"action":  queryInfo.Action,
				}).
				Set(utils.BoolToFloat64(queryInfo.Success))
		}
	}

	return []prometheus.Collector{querySuccessfulGauge}
}
