package durosensurer

type (
	QoSPolicyDef struct {
		Name        string `json:"name"`
		Description string
		Limit       QoSPolicyLimit
	}

	// QoSPolicyLimit is the quality of service policy limit.
	// Only one field is being set. The other fields will be null.
	QoSPolicyLimit struct {
		IOPS      *QoSPolicyLimitIOPS
		Bandwidth *QoSPolicyLimitBandwidth
		IOPSPerGB *QoSPolicyLimitIOPSPerGB
	}

	// QoSPolicyLimitIOPS represents a limit of IOPS. Must be a power of 2, but at least 256. 0 represents unlimited IOPS.
	QoSPolicyLimitIOPS struct {
		Read  uint32
		Write uint32
	}
	// QoSPolicyLimitBandwidth limits the bandwidth in units of full MB/s. 0 reprensents an unlimited bandwidth.
	QoSPolicyLimitBandwidth struct {
		Read  uint32
		Write uint32
	}
	// QoSPolicyLimitIOPSPerGB represents a limit of IOPS per GB volume size. 0 represents unlimited IOPS.
	QoSPolicyLimitIOPSPerGB struct {
		Read  uint32
		Write uint32
	}
)
