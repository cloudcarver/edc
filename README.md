# EDC (Extremely Dope Code)

## Conf

Read config from configuration file OR from environment variables, then unmarshal it into a struct.

1. You can use config file to set up your application. This step is optional.
    ```yaml
    pg:
        host: 127.0.0.1
        port: 5432
    secret:
        authorizedKey: "key"
    ```

2. Use `FetchConfig` to load the config file.
    ```go
    import "github.com/cloudcarver/edc/conf"

    type PG struct {
        Host string `yaml:"host"`
        Port int    `yaml:"port"`
    }

    type Secret struct {
        AuthorizedKey string `yaml:"authorizedKey"`
    }

    type Config struct {
        PG     PG     `yaml:"pg"`
        Secret Secret `yaml:"secret"`
    }

    func main() {
        config := Config{}
        err := conf.FetchConfig("conf.yaml", "MYAPP", &config)
        if err != nil {
            panic(err)
        }
        ...
    }
    ```

3. Start the application.
    ```shell
    MYAPP_PG_HOST=myapp.local go run main.go
    ```
