use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

use actix_web::{App, HttpServer};
use async_trait::async_trait;
use pact_verifier::callback_executors::{NullRequestFilterExecutor, ProviderStateExecutor};
use pact_verifier::{FilterInfo, ProviderInfo, ProviderTransport, VerificationOptions, PactSource};
use portpicker::pick_unused_port;

use shipping::shipping_service::{get_quote, ship_order};
use serde_json::Value;
use pact_models::prelude::ProviderState;

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn verify_shipping_pact() {
    // Pick ports for provider and quote stub
    let port = pick_unused_port().expect("No free port for provider");
    let quote_port = pick_unused_port().expect("No free port for quote service");
    
    // Start the shipping service HTTP server in the background.
    // Start stub quote service that returns fixed float value
    let quote_server = HttpServer::new(|| App::new().default_service(actix_web::web::to(|| async { "5.99" })))
        .bind(("127.0.0.1", quote_port)).expect("bind quote").run();
    let _quote_handle = tokio::spawn(quote_server);

    // point provider to stub quote service
    std::env::set_var("QUOTE_ADDR", format!("http://127.0.0.1:{}", quote_port));

    let server = HttpServer::new(|| {
        App::new()
            .service(get_quote)
            .service(ship_order)
    })
    .bind(("127.0.0.1", port)).expect("failed to bind port")
    .run();

    let srv_handle = tokio::spawn(server);

    // drop _quote_handle when test ends
    let _ = _quote_handle;

    // Build ProviderInfo for pact verifier.
    let provider_info = ProviderInfo {
        name: "ShippingService".to_string(),
        host: "127.0.0.1".to_string(),
        transports: vec![ProviderTransport {
            transport: "http".to_string(),
            port: Some(port),
            path: None,
            scheme: Some("http".to_string()),
        }],
        ..Default::default()
    };

    // Locate the pact file relative to the crate root.
    let pact_path: PathBuf = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../frontend/pacts/Frontend-ShippingService.json");

    // Prepare options – we don't need any special filters or headers.
    let verification_options = VerificationOptions::<NullRequestFilterExecutor> {
        request_filter: None,
        disable_ssl_verification: false,
        request_timeout: 5000,
        custom_headers: Default::default(),
        coloured_output: false,
        no_pacts_is_error: true,
        exit_on_first_failure: false,
        run_last_failed_only: false,
    };

    // Simple provider state executor that does nothing and always succeeds.
    #[derive(Debug, Clone)]
    struct NoOpProviderState;

    #[async_trait]
    impl ProviderStateExecutor for NoOpProviderState {
        async fn call(
            self: Arc<Self>,
            _interaction_id: Option<String>,
            _provider_state: &ProviderState,
            _setup: bool,
            _client: Option<&reqwest::Client>,
        ) -> anyhow::Result<HashMap<String, Value>> {
            Ok(HashMap::new())
        }

        fn teardown(self: &Self) -> bool {
            true
        }
    }

    let provider_state_executor = Arc::new(NoOpProviderState);

    // Run the verifier against the pact file.
    let result = pact_verifier::verify_provider_async(
        provider_info,
        vec![PactSource::File(pact_path.to_string_lossy().to_string())],
        FilterInfo::None,
        vec![],
        &verification_options,
        None,
        &provider_state_executor,
        None,
    )
    .await
    .expect("Pact verification process errored");

    // Assert all interactions passed.
    assert!(result.result, "Pact verification failed, see output for details");

    // Shut down the server task.
    srv_handle.abort();
}
