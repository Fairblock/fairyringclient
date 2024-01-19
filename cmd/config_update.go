package cmd

import (
	"fairyringclient/config"
	"fmt"
	"github.com/spf13/cobra"
	"net/url"
)

// configUpdateCmd represents the config update command
var configUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update FairyRing Client config file",
	Long:  `Update FairyRing Client config file`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.ReadConfigFromFile()
		if err != nil {
			fmt.Printf("Error loading config from file: %s\n", err.Error())
			return
		}

		chainID, _ := cmd.Flags().GetString("chain-id")
		chainDenom, _ := cmd.Flags().GetString("denom")
		chainIP, _ := cmd.Flags().GetString("ip")
		chainProtocol, _ := cmd.Flags().GetString("protocol")
		chainGrpcPort, _ := cmd.Flags().GetUint64("grpc-port")
		chainPort, _ := cmd.Flags().GetUint64("port")
		apiURL, _ := cmd.Flags().GetString("api-url")
		pauseThreshold, _ := cmd.Flags().GetUint64("pause-threshold")
		metricsPort, _ := cmd.Flags().GetUint64("metrics-port")

		cfg.FairyRingNode = config.Node{
			Protocol: chainProtocol,
			IP:       chainIP,
			Port:     chainPort,
			GRPCPort: chainGrpcPort,
			Denom:    chainDenom,
			ChainID:  chainID,
		}

		_, err = url.ParseRequestURI(apiURL)
		if err != nil {
			fmt.Printf("Invalid API URL Provided '%s', Error: %s\n", apiURL, err.Error())
			return
		}

		cfg.ShareAPIUrl = apiURL
		cfg.InvalidSharePauseThreshold = pauseThreshold
		cfg.MetricsPort = metricsPort

		if err = cfg.SaveConfig(); err != nil {
			fmt.Printf("Error saving updated config to system: %s\n", err.Error())
			return
		}

		fmt.Println("Successfully Updated config!")
	},
}

func init() {
	cfg, err := config.ReadConfigFromFile()
	if err != nil {
		fmt.Printf("Error loading config from file: %s\n", err.Error())
		return
	}

	configUpdateCmd.Flags().String("chain-id", cfg.FairyRingNode.ChainID, "Update config chain id")
	configUpdateCmd.Flags().String("denom", cfg.FairyRingNode.Denom, "Update config denom")
	configUpdateCmd.Flags().Uint64("grpc-port", cfg.FairyRingNode.GRPCPort, "Update config grpc-port")
	configUpdateCmd.Flags().String("ip", cfg.FairyRingNode.IP, "Update config node ip address")
	configUpdateCmd.Flags().Uint64("port", cfg.FairyRingNode.Port, "Update config node port")
	configUpdateCmd.Flags().String("protocol", cfg.FairyRingNode.Protocol, "Update config node protocol")
	configUpdateCmd.Flags().String("api-url", cfg.ShareAPIUrl, "Update config API URL")
	configUpdateCmd.Flags().Uint64("pause-threshold", cfg.InvalidSharePauseThreshold, "Update the threshold of when the client pause if number of invalid share in a row reaches threshold")
	configUpdateCmd.Flags().Uint64("metrics-port", cfg.MetricsPort, "Update the port of metrics listen to")
}
