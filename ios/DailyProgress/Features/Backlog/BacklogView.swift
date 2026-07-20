import SwiftUI

// MARK: - BacklogView

/// Backlog tab: two sections (This week / Next week) with adopt and shuttle actions.
///
/// Adopt always targets the real current date — not the Today tab's viewed date —
/// matching Qt's time.Now() semantic.  Shuttle (move) toggles an item between
/// sections; NOT_FOUND races surface as a toast, never an error state.
struct BacklogView: View {
    let appState: AppState
    @State private var store: BacklogStore

    init(appState: AppState) {
        self.appState = appState
        // core is guaranteed non-nil when BacklogView initialises: DailyProgressApp only
        // renders RootTabView (and thus tab views) after openCore() succeeds.
        _store = State(initialValue: BacklogStore(
            core: appState.core!,
            onMutation: { appState.bumpDataVersion() }  // notify Today tab on adopt (I2)
        ))
    }

    var body: some View {
        NavigationStack {
            Group {
                if store.isLoading && store.backlog == nil {
                    ProgressView("Loading\u{2026}")
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else if store.backlog == nil, let errMsg = store.errorMessage {
                    ContentUnavailableView(
                        "Failed to Load",
                        systemImage: "exclamationmark.triangle",
                        description: Text(errMsg)
                    )
                } else {
                    backlogContent
                }
            }
            .navigationTitle("Backlog")
            .navigationBarTitleDisplayMode(.inline)
            .toast(store.toast)
            // Show mutation errors as alerts when content is already visible (rule 4).
            .alert("Error", isPresented: Binding(
                get: { store.backlog != nil && store.errorMessage != nil },
                set: { if !$0 { store.errorMessage = nil } }
            )) {
                Button("OK", role: .cancel) { store.errorMessage = nil }
            } message: {
                if let msg = store.errorMessage { Text(msg) }
            }
        }
        // Refresh when another tab mutates shared data (bumpDataVersion).
        // Evening check-in action 4 (backlog) and Today "Move to Backlog" both add items here.
        .task(id: appState.dataVersion) {
            await store.refresh()
        }
    }

    // MARK: - Backlog content

    private var backlogContent: some View {
        let current = store.backlog?.current ?? []
        let nextWeek = store.backlog?.nextWeek ?? []

        return List {
            Section("This week") {
                ForEach(Array(current.enumerated()), id: \.offset) { _, text in
                    backlogRow(text: text, isCurrentWeek: true)
                }
            }
            Section("Next week") {
                ForEach(Array(nextWeek.enumerated()), id: \.offset) { _, text in
                    backlogRow(text: text, isCurrentWeek: false)
                }
            }
        }
        .listStyle(.insetGrouped)
        .refreshable { await store.refresh() }
        // Show the empty state only when data is loaded and both sections are empty.
        .overlay {
            if let b = store.backlog, b.current.isEmpty && b.nextWeek.isEmpty {
                ContentUnavailableView("Nothing in the backlog", systemImage: "tray")
            }
        }
    }

    // MARK: - Row

    /// One backlog row with swipe and context-menu actions.
    ///
    /// Section identity: rows are keyed by (section, index) via the ForEach offset
    /// because duplicate texts across sections are legal (e.g. the same task text
    /// can be in both current and next-week if data is written directly).
    ///
    /// Shuttle direction:
    ///   isCurrentWeek = true  → trailing swipe moves to Next week (toNextWeek: true)
    ///   isCurrentWeek = false → trailing swipe moves to This week  (toNextWeek: false)
    @ViewBuilder
    private func backlogRow(text: String, isCurrentWeek: Bool) -> some View {
        let shuttleLabel = isCurrentWeek ? "Next week" : "This week"
        let shuttleIcon  = isCurrentWeek ? "arrow.right" : "arrow.left"

        Text(text)
            // Leading swipe: "Plan Today" (green, adopt into current day's plan)
            .swipeActions(edge: .leading) {
                Button {
                    Task { await store.adopt(text: text) }
                } label: {
                    Label("Plan Today", systemImage: "arrow.down.circle")
                }
                .tint(.green)
            }
            // Trailing swipe: shuttle to the other section
            .swipeActions(edge: .trailing) {
                Button {
                    Task { await store.move(text: text, toNextWeek: isCurrentWeek) }
                } label: {
                    Label(shuttleLabel, systemImage: shuttleIcon)
                }
            }
            // Context menu duplicates both actions for accessibility
            .contextMenu {
                Button {
                    Task { await store.adopt(text: text) }
                } label: {
                    Label("Plan Today", systemImage: "arrow.down.circle")
                }
                Button {
                    Task { await store.move(text: text, toNextWeek: isCurrentWeek) }
                } label: {
                    Label(
                        isCurrentWeek ? "Move to Next Week" : "Move to This Week",
                        systemImage: shuttleIcon
                    )
                }
            }
    }
}
