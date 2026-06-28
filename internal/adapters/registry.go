package adapters

var builtInAdapters = []Adapter{
	MongoSH{},
	PSQL{},
	RedisCLI{},
}

func SupportedManagedCLIs() []string {
	managedCLIs := make([]string, 0, len(builtInAdapters))
	for _, adapter := range builtInAdapters {
		managedCLIs = append(managedCLIs, adapter.Name())
	}
	return managedCLIs
}

func IsSupported(managedCLI string) bool {
	_, ok := Get(managedCLI)
	return ok
}

func Get(managedCLI string) (Adapter, bool) {
	for _, adapter := range builtInAdapters {
		if adapter.Name() == managedCLI {
			return adapter, true
		}
	}
	return nil, false
}
