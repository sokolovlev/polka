//! Configuration from environment variables, validated at startup so that a
//! misconfigured service fails fast.

use std::net::SocketAddr;
use std::time::Duration;

use serde::de::DeserializeOwned;
use serde::Deserialize;

/// Error returned by [`load`].
#[derive(Debug, thiserror::Error)]
pub enum ConfigError {
    #[error("parse config: {0}")]
    Parse(#[from] envy::Error),
    #[error("invalid config: {0}")]
    Invalid(String),
}

/// Implemented by configs with checks beyond parsing.
pub trait Validate {
    /// Returns every problem found, joined into one message.
    fn validate(&self) -> Result<(), String>;
}

/// Parses environment variables into `T` (field `admin_addr` reads
/// `ADMIN_ADDR`), then validates it.
pub fn load<T: DeserializeOwned + Validate>() -> Result<T, ConfigError> {
    from_iter(std::env::vars())
}

/// Like [`load`], from an explicit set of variables (for tests).
pub fn from_iter<T, I>(vars: I) -> Result<T, ConfigError>
where
    T: DeserializeOwned + Validate,
    I: IntoIterator<Item = (String, String)>,
{
    let cfg: T = envy::from_iter(vars)?;
    cfg.validate().map_err(ConfigError::Invalid)?;
    Ok(cfg)
}

/// Settings every Rust service has. Load it with its own [`load`] call, next
/// to the service's config: envy cannot parse non-string fields through
/// `#[serde(flatten)]` (it hands every value to a flattened struct as a
/// string).
#[derive(Debug, Clone, Deserialize)]
pub struct Common {
    /// Admin HTTP server: `/healthz`, `/readyz`, `/version`.
    #[serde(default = "default_admin_addr")]
    pub admin_addr: SocketAddr,
    /// `tracing` filter, e.g. `info` or `info,inventory=debug`.
    #[serde(default = "default_log_level")]
    pub log_level: String,
    /// Seconds to finish in-flight work after SIGTERM (spec 8.2: up to 25 s).
    #[serde(default = "default_shutdown_timeout_sec")]
    pub shutdown_timeout_sec: u64,
}

impl Common {
    pub fn shutdown_timeout(&self) -> Duration {
        Duration::from_secs(self.shutdown_timeout_sec)
    }
}

impl Validate for Common {
    fn validate(&self) -> Result<(), String> {
        if !(1..=25).contains(&self.shutdown_timeout_sec) {
            return Err(format!(
                "SHUTDOWN_TIMEOUT_SEC: {} not in 1..25",
                self.shutdown_timeout_sec
            ));
        }
        Ok(())
    }
}

fn default_admin_addr() -> SocketAddr {
    SocketAddr::from(([0, 0, 0, 0], 8081))
}

fn default_log_level() -> String {
    "info".to_owned()
}

fn default_shutdown_timeout_sec() -> u64 {
    25
}

#[cfg(test)]
mod tests {
    use super::*;

    #[derive(Debug, Deserialize)]
    struct ServiceConfig {
        database_url: String,
        #[serde(default)]
        max_items: u32,
    }

    impl Validate for ServiceConfig {
        fn validate(&self) -> Result<(), String> {
            if self.max_items > 100 {
                return Err("MAX_ITEMS: must be <= 100".to_owned());
            }
            Ok(())
        }
    }

    fn vars(pairs: &[(&str, &str)]) -> Vec<(String, String)> {
        pairs
            .iter()
            .map(|(k, v)| ((*k).to_owned(), (*v).to_owned()))
            .collect()
    }

    #[test]
    fn common_defaults_apply() {
        let cfg: Common = from_iter(vars(&[])).unwrap();
        assert_eq!(cfg.admin_addr, default_admin_addr());
        assert_eq!(cfg.log_level, "info");
        assert_eq!(cfg.shutdown_timeout(), Duration::from_secs(25));
    }

    #[test]
    fn common_parses_values() {
        let cfg: Common = from_iter(vars(&[
            ("ADMIN_ADDR", "127.0.0.1:9000"),
            ("SHUTDOWN_TIMEOUT_SEC", "10"),
        ]))
        .unwrap();
        assert_eq!(cfg.admin_addr, SocketAddr::from(([127, 0, 0, 1], 9000)));
        assert_eq!(cfg.shutdown_timeout(), Duration::from_secs(10));
    }

    #[test]
    fn service_config_reads_its_fields() {
        let cfg: ServiceConfig = from_iter(vars(&[
            ("DATABASE_URL", "postgres://x"),
            ("MAX_ITEMS", "5"),
        ]))
        .unwrap();
        assert_eq!(cfg.database_url, "postgres://x");
        assert_eq!(cfg.max_items, 5);
    }

    #[test]
    fn missing_required_fails() {
        let err = from_iter::<ServiceConfig, _>(vars(&[])).unwrap_err();
        assert!(err.to_string().contains("database_url"), "{err}");
    }

    #[test]
    fn out_of_range_fails_validation() {
        let err = from_iter::<Common, _>(vars(&[("SHUTDOWN_TIMEOUT_SEC", "60")])).unwrap_err();
        assert!(matches!(err, ConfigError::Invalid(_)), "{err}");
        let err = from_iter::<ServiceConfig, _>(vars(&[
            ("DATABASE_URL", "postgres://x"),
            ("MAX_ITEMS", "500"),
        ]))
        .unwrap_err();
        assert!(matches!(err, ConfigError::Invalid(_)), "{err}");
    }

    #[test]
    fn unparsable_value_fails() {
        let err = from_iter::<Common, _>(vars(&[("ADMIN_ADDR", "not-an-addr")])).unwrap_err();
        assert!(matches!(err, ConfigError::Parse(_)), "{err}");
    }
}
