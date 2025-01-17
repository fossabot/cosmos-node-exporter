package upgrades

import (
	"main/pkg/constants"
	"main/pkg/cosmovisor"
	"main/pkg/grpc"
	"main/pkg/query_info"
	"main/pkg/tendermint"
	"main/pkg/utils"
	"net/url"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

type UpgradesQuerier struct {
	Logger     zerolog.Logger
	Cosmovisor *cosmovisor.Cosmovisor
	Grpc       *grpc.Grpc
	Tendermint *tendermint.TendermintRPC
}

func NewUpgradesQuerier(
	logger *zerolog.Logger,
	cosmovisor *cosmovisor.Cosmovisor,
	grpc *grpc.Grpc,
	tendermint *tendermint.TendermintRPC,
) *UpgradesQuerier {
	return &UpgradesQuerier{
		Logger:     logger.With().Str("component", "upgrades_querier").Logger(),
		Cosmovisor: cosmovisor,
		Grpc:       grpc,
		Tendermint: tendermint,
	}
}

func (u *UpgradesQuerier) Enabled() bool {
	return u.Grpc != nil
}

func (u *UpgradesQuerier) Name() string {
	return "upgrades-querier"
}

func (u *UpgradesQuerier) Get() ([]prometheus.Collector, []query_info.QueryInfo) {
	grpcQuery := query_info.QueryInfo{
		Module:  "grpc",
		Action:  "get_upgrade_plan",
		Success: false,
	}

	upgrade, err := u.Grpc.GetUpgradePlan()
	if err != nil {
		u.Logger.Err(err).Msg("Could not get latest upgrade plan from gRPC")
		return []prometheus.Collector{}, []query_info.QueryInfo{grpcQuery}
	}

	grpcQuery.Success = true
	isUpgradePresent := upgrade != nil

	upcomingUpgradePresent := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: constants.MetricsPrefix + "upgrade_coming",
			Help: "Is future upgrade planned?",
		},
		[]string{},
	)

	upcomingUpgradePresent.
		With(prometheus.Labels{}).
		Set(utils.BoolToFloat64(isUpgradePresent))

	queries := []prometheus.Collector{upcomingUpgradePresent}
	queryInfos := []query_info.QueryInfo{grpcQuery}

	if !isUpgradePresent {
		return queries, queryInfos
	}

	upgradeInfoGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: constants.MetricsPrefix + "upgrade_info",
			Help: "Future upgrade info",
		},
		[]string{"name", "info"},
	)

	upgradeInfoGauge.
		With(prometheus.Labels{"name": upgrade.Name, "info": upgrade.Info}).
		Set(utils.BoolToFloat64(isUpgradePresent))
	queries = append(queries, upgradeInfoGauge)

	upgradeEstimatedTimeGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: constants.MetricsPrefix + "upgrade_estimated_time",
			Help: "Estimated upgrade time, as Unix timestamp",
		},
		[]string{"name", "info"},
	)

	upgradeTime := upgrade.Time
	if upgradeTime.IsZero() {
		if u.Tendermint == nil {
			u.Logger.Warn().
				Msg("Tendermint RPC not initialized and upgrade time is not specified, not returning upgrade time.")
			return queries, queryInfos
		}

		tendermintQuery := query_info.QueryInfo{
			Module:  "tendermint",
			Action:  "tendermint_get_upgrade_time",
			Success: false,
		}

		upgradeTime, err = u.Tendermint.GetEstimateTimeTillBlock(upgrade.Height)
		if err != nil {
			u.Logger.Err(err).Msg("Could not get estimated upgrade time")
			queryInfos = append(queryInfos, tendermintQuery)
			return queries, queryInfos
		}
		tendermintQuery.Success = true
		queryInfos = append(queryInfos, tendermintQuery)
	}

	upgradeEstimatedTimeGauge.
		With(prometheus.Labels{"name": upgrade.Name, "info": upgrade.Info}).
		Set(float64(upgradeTime.Unix()))
	queries = append(queries, upgradeEstimatedTimeGauge)

	if u.Cosmovisor == nil {
		u.Logger.Warn().
			Msg("Cosmovisor not initialized, not returning binary presence.")
		return queries, queryInfos
	}

	cosmovisorGetUpgradesQueryInfo := query_info.QueryInfo{
		Action:  "cosmovisor_get_upgrades",
		Success: false,
	}

	upgrades, err := u.Cosmovisor.GetUpgrades()
	if err != nil {
		u.Logger.Error().Err(err).Msg("Could not get Cosmovisor upgrades")
		queryInfos = append(queryInfos, cosmovisorGetUpgradesQueryInfo)
		return []prometheus.Collector{}, queryInfos
	}

	cosmovisorGetUpgradesQueryInfo.Success = true
	queryInfos = append(queryInfos, cosmovisorGetUpgradesQueryInfo)

	upgradeBinaryPresentGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: constants.MetricsPrefix + "upgrade_binary_present",
			Help: "Is upgrade binary present?",
		},
		[]string{"name", "info"},
	)

	// From cosmovisor docs:
	// The name variable in upgrades/<name> is the lowercased URI-encoded name
	// of the upgrade as specified in the upgrade module plan.
	upgradeName := strings.ToLower(upgrade.Name)
	upgradeName = url.QueryEscape(upgradeName)

	upgradeBinaryPresentGauge.
		With(prometheus.Labels{"name": upgrade.Name, "info": upgrade.Info}).
		Set(utils.BoolToFloat64(upgrades.HasUpgrade(upgradeName)))
	queries = append(queries, upgradeBinaryPresentGauge)

	return queries, queryInfos
}
