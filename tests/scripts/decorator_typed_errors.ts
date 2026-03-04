// expect_compile_error: decorator must be a callable expression
// Type errors with various non-callable types as decorators
interface Config {
  host: string;
  port: number;
}

const config: Config = { host: "localhost", port: 8080 };

@config
class Server {}
