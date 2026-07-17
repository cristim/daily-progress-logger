import SwiftUI

@main
struct DailyProgressApp: App {
    @State private var appState = AppState()

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
            .onChange(of: appState.core != nil ? 1 : 0) { _, _ in
                // Core opened; nothing extra needed here.
            }
        }
    }
}
