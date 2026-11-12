fn main() {
    // The two commands the Agent Desk page may call; capabilities/page.json
    // grants exactly these to the page served on 127.0.0.1.
    tauri_build::try_build(
        tauri_build::Attributes::new()
            .app_manifest(tauri_build::AppManifest::new().commands(&["notify", "set_badge"])),
    )
    .expect("tauri build");
}
