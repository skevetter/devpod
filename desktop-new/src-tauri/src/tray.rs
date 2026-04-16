use crate::SharedState;
use log::error;
use tauri::menu::{MenuBuilder, MenuItemBuilder, PredefinedMenuItem};
use tauri::tray::TrayIconBuilder;
use tauri::{AppHandle, Manager};

pub fn setup_tray(app: &AppHandle) -> Result<(), Box<dyn std::error::Error>> {
    let show = MenuItemBuilder::with_id("show", "Show DevPod").build(app)?;
    let hide = MenuItemBuilder::with_id("hide", "Hide").build(app)?;
    let quit = MenuItemBuilder::with_id("quit", "Quit DevPod").build(app)?;

    let menu = MenuBuilder::new(app)
        .item(&show)
        .item(&hide)
        .separator()
        .item(&quit)
        .build()?;

    let tray = TrayIconBuilder::new()
        .icon(app.default_window_icon().cloned().unwrap())
        .tooltip("DevPod Desktop")
        .menu(&menu)
        .on_menu_event(move |app, event| match event.id().as_ref() {
            "show" => {
                if let Some(window) = app.get_webview_window("main") {
                    let _ = window.show();
                    let _ = window.set_focus();
                }
            }
            "hide" => {
                if let Some(window) = app.get_webview_window("main") {
                    let _ = window.hide();
                }
            }
            "quit" => {
                app.exit(0);
            }
            id => {
                if let Some(ws_id) = id.strip_prefix("ws-open-") {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.show();
                        let _ = window.set_focus();
                        let _ = window.eval(&format!(
                            "window.location.href = '/workspaces/{}'",
                            ws_id
                        ));
                    }
                }
            }
        })
        .build(app)?;

    // Periodically rebuild the tray menu with workspace list
    let app_handle = app.clone();
    tokio::spawn(async move {
        loop {
            tokio::time::sleep(std::time::Duration::from_secs(5)).await;
            if let Err(e) = rebuild_tray_menu(&app_handle, &tray).await {
                error!("Failed to rebuild tray menu: {}", e);
            }
        }
    });

    Ok(())
}

async fn rebuild_tray_menu(
    app: &AppHandle,
    tray: &tauri::tray::TrayIcon,
) -> Result<(), Box<dyn std::error::Error>> {
    let state = app.state::<SharedState>();
    let state = state.read().await;
    let workspaces = state.workspace_list();

    let mut builder = MenuBuilder::new(app);

    let count = workspaces.len();
    let status_label = if count == 0 {
        "No workspaces".to_string()
    } else {
        format!(
            "{} workspace{}",
            count,
            if count == 1 { "" } else { "s" }
        )
    };

    builder = builder.item(
        &MenuItemBuilder::with_id("status", &status_label)
            .enabled(false)
            .build(app)?,
    );

    // Add workspace entries for quick navigation
    if !workspaces.is_empty() {
        builder = builder.item(&PredefinedMenuItem::separator(app)?);
        for ws in workspaces.iter().take(10) {
            let item_id = format!("ws-open-{}", ws.id);
            builder = builder.item(
                &MenuItemBuilder::with_id(item_id, &format!("  {}", ws.id)).build(app)?,
            );
        }
        if count > 10 {
            builder = builder.item(
                &MenuItemBuilder::with_id("more", &format!("  ... and {} more", count - 10))
                    .enabled(false)
                    .build(app)?,
            );
        }
    }

    builder = builder.item(&PredefinedMenuItem::separator(app)?);
    builder = builder.item(&MenuItemBuilder::with_id("show", "Show DevPod").build(app)?);
    builder = builder.item(&MenuItemBuilder::with_id("hide", "Hide").build(app)?);
    builder = builder.item(&PredefinedMenuItem::separator(app)?);
    builder = builder.item(&MenuItemBuilder::with_id("quit", "Quit DevPod").build(app)?);

    let menu = builder.build()?;
    tray.set_menu(Some(menu))?;
    tray.set_tooltip(Some(&format!("DevPod — {}", status_label)))?;

    Ok(())
}
