package plugin

import (
	"github.com/hashicorp/hcl/v2/hclsimple"
	"log"
	"testing"
)

var config RedisConfig

type RedisConfig struct {
	Address       string `hcl:"address"`
	Password      string `hcl:"password"`
	MaxActiveConn int    `hcl:"max_active_conn"`
	MaxIdleConn   int    `hcl:"max_idle_conn"`
}

func Test_decodeFile(t *testing.T) {
	err := hclsimple.DecodeFile("./../config/redis_conf.hcl", nil, &config)
	if err != nil {
		log.Fatalf("Failed to load configuration: %s", err)
	}
	log.Printf("Configuration is %#v", config)
}
