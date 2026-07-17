import SwiftUI

// MARK: - MoreView (tab 4)

struct MoreView: View {
    let appState: AppState

    var body: some View {
        NavigationStack {
            List {
                NavigationLink {
                    ProjectsView(appState: appState)
                } label: {
                    Label("Projects", systemImage: "folder")
                }
                NavigationLink {
                    RecurringView(appState: appState)
                } label: {
                    Label("Recurring Tasks", systemImage: "repeat")
                }
                NavigationLink {
                    RecycleView(appState: appState)
                } label: {
                    Label("Recycle Bin", systemImage: "trash")
                }
                NavigationLink {
                    SyncView(appState: appState)
                } label: {
                    Label("Sync & Account", systemImage: "arrow.triangle.2.circlepath")
                }
                NavigationLink {
                    SettingsView(appState: appState)
                } label: {
                    Label("Settings", systemImage: "gearshape")
                }
            }
            .navigationTitle("More")
        }
    }
}
