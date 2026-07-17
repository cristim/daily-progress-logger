import SwiftUI

@main
struct DailyProgressApp: App {
    @State private var appState = AppState()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            Group {
                if let error = appState.launchError {
                    ContentUnavailableView(
                        "Failed to Open",
                        systemImage: "exclamationmark.triangle",
                        description: Text(error)
                    )
                } else if appState.core == nil {
                    ProgressView("Opening…")
                } else {
                    RootTabView(appState: appState)
                }
            }
            .task {
                await appState.openCore()
                await appState.refreshDuePrompts()
            }
            // Re-evaluate due prompts whenever the app returns to the foreground
            // so the coordinator can surface any newly-due check-ins.
            .onChange(of: scenePhase) { _, phase in
                if phase == .active {
                    Task { await appState.refreshDuePrompts() }
                }
            }
        }
    }
}
