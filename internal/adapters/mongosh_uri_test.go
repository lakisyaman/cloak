package adapters

import "testing"

func TestApplyMongoDefaultDatabase(t *testing.T) {
	tests := []struct {
		name            string
		rawURI          string
		defaultDatabase string
		want            string
	}{
		{
			name:            "standard uri without path",
			rawURI:          "mongodb://localhost:27017",
			defaultDatabase: "app",
			want:            "mongodb://localhost:27017/app",
		},
		{
			name:            "standard uri with query",
			rawURI:          "mongodb://localhost:27017?retryWrites=true",
			defaultDatabase: "app",
			want:            "mongodb://localhost:27017/app?retryWrites=true",
		},
		{
			name:            "srv uri without path",
			rawURI:          "mongodb+srv://cluster.example.com",
			defaultDatabase: "app",
			want:            "mongodb+srv://cluster.example.com/app",
		},
		{
			name:            "srv uri with query",
			rawURI:          "mongodb+srv://cluster.example.com?retryWrites=true&w=majority",
			defaultDatabase: "app",
			want:            "mongodb+srv://cluster.example.com/app?retryWrites=true&w=majority",
		},
		{
			name:            "replica set seed list without path",
			rawURI:          "mongodb://h1.example.com:27017,h2.example.com:27017",
			defaultDatabase: "app",
			want:            "mongodb://h1.example.com:27017,h2.example.com:27017/app",
		},
		{
			name:            "embedded userinfo survives default database insertion",
			rawURI:          "mongodb://user:pass@localhost:27017?authSource=admin",
			defaultDatabase: "app",
			want:            "mongodb://user:pass@localhost:27017/app?authSource=admin",
		},
		{
			name:            "existing path wins",
			rawURI:          "mongodb+srv://cluster.example.com/existing?retryWrites=true",
			defaultDatabase: "app",
			want:            "mongodb+srv://cluster.example.com/existing?retryWrites=true",
		},
		{
			name:            "empty default database leaves uri unchanged",
			rawURI:          "mongodb://localhost:27017?retryWrites=true",
			defaultDatabase: "",
			want:            "mongodb://localhost:27017?retryWrites=true",
		},
		{
			name:            "slash path is treated as absent",
			rawURI:          "mongodb://localhost:27017/?retryWrites=true",
			defaultDatabase: "app",
			want:            "mongodb://localhost:27017/app?retryWrites=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ApplyMongoDefaultDatabase(tt.rawURI, tt.defaultDatabase)
			if err != nil {
				t.Fatalf("ApplyMongoDefaultDatabase returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestApplyMongoDefaultDatabaseRejectsUnsupportedSchemes(t *testing.T) {
	_, err := ApplyMongoDefaultDatabase("postgres://localhost/app", "app")
	if err == nil {
		t.Fatalf("expected unsupported scheme error")
	}
}
