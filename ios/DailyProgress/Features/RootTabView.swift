import SwiftUI

// MARK: - RootTabView

struct RootTabView: View {
    let appState: AppState

    var body: some View {
        TabView {
            TodayView(appState: appState)
                .tabItem {
                    Label("Today", systemImage: "checkmark.circle")
                }

            WeekView(appState: appState)
                .tabItem {
                    Label("Week", systemImage: "calendar")
                }

            BacklogView(appState: appState)
                .tabItem {
                    Label("Backlog", systemImage: "tray.full")
                }

            MoreView(appState: appState)
                .tabItem {
                    Label("More", systemImage: "ellipsis.circle")
                }
        }
    }
}
