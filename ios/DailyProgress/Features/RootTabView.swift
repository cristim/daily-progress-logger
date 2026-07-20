import SwiftUI

// MARK: - RootTabView

struct RootTabView: View {
    let appState: AppState
    @State private var coordinator = CheckinCoordinator()
    @State private var checkinStore: CheckinStore?

    // Manual trigger state for Today toolbar
    @State private var showMorningManual = false
    @State private var showEveningManual = false

    var body: some View {
        @Bindable var bindCoord = coordinator

        TabView {
            TodayView(appState: appState, coordinator: coordinator)
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
        // Lazily create CheckinStore once core is available
        .task(id: appState.core != nil ? 1 : 0) {
            if let core = appState.core {
                checkinStore = CheckinStore(core: core)
            }
        }
        // Route due prompts through coordinator whenever duePrompts updates
        .onChange(of: appState.duePrompts) { _, prompts in
            coordinator.process(duePrompts: prompts)
        }
        // Create a fresh CheckinStore each time the coordinator schedules a new prompt
        // so each session starts with clean state (no stale toggles from a prior session).
        .onChange(of: coordinator.scheduledPrompt) { _, prompt in
            if prompt != nil, let core = appState.core {
                checkinStore = CheckinStore(core: core)
            }
        }
        // Scheduled sheet: driven by coordinator.scheduledPrompt
        .sheet(item: $bindCoord.scheduledPrompt, onDismiss: { coordinator.dismissCurrent() }) { prompt in
            scheduledSheet(for: prompt)
        }
        // Manual morning sheet (Today toolbar)
        .sheet(isPresented: $showMorningManual) {
            if let store = checkinStore {
                MorningCheckinSheet(
                    store: store,
                    appState: appState,
                    presentation: .manual,
                    onComplete: {},
                    onSnooze: {
                        // Manual snooze still suppresses the auto-prompt
                        if let prompt = DuePrompt.synthetic(.morning) {
                            coordinator.snooze(prompt: prompt)
                        }
                    },
                    onSkipOrClose: {}  // manual Close does no bookkeeping
                )
            }
        }
        // Manual evening sheet (Today toolbar)
        .sheet(isPresented: $showEveningManual) {
            if let store = checkinStore {
                EveningCheckinSheet(
                    store: store,
                    appState: appState,
                    presentation: .manual,
                    onComplete: {},
                    onSnooze: {
                        if let prompt = DuePrompt.synthetic(.evening) {
                            coordinator.snooze(prompt: prompt)
                        }
                    },
                    onSkipOrClose: {}  // manual Close does no bookkeeping
                )
            }
        }
        .environment(\.showMorningCheckin, {
            if let core = appState.core { checkinStore = CheckinStore(core: core) }
            showMorningManual = true
        })
        .environment(\.showEveningCheckin, {
            if let core = appState.core { checkinStore = CheckinStore(core: core) }
            showEveningManual = true
        })
    }

    // MARK: - Scheduled sheet routing

    @ViewBuilder
    private func scheduledSheet(for prompt: DuePrompt) -> some View {
        if let store = checkinStore {
            switch prompt.id {
            case .morning:
                MorningCheckinSheet(
                    store: store,
                    appState: appState,
                    presentation: .scheduled,
                    onComplete: {},
                    onSnooze: { coordinator.snooze(prompt: prompt) },
                    onSkipOrClose: { coordinator.skipToday(prompt: prompt) }
                )
            case .evening:
                EveningCheckinSheet(
                    store: store,
                    appState: appState,
                    presentation: .scheduled,
                    onComplete: {},
                    onSnooze: { coordinator.snooze(prompt: prompt) },
                    onSkipOrClose: { coordinator.skipToday(prompt: prompt) }
                )
            default:
                // Phase B: weekReview (0), weeklyPlan (1), weeklySummary (4)
                EmptyView()
            }
        }
    }
}

// MARK: - Environment keys for manual triggers

/// Allows TodayView (or any descendant) to trigger check-in sheets
/// without needing a direct reference to coordinator or RootTabView state.
private struct ShowMorningCheckinKey: EnvironmentKey {
    static let defaultValue: () -> Void = {}
}

private struct ShowEveningCheckinKey: EnvironmentKey {
    static let defaultValue: () -> Void = {}
}

extension EnvironmentValues {
    var showMorningCheckin: () -> Void {
        get { self[ShowMorningCheckinKey.self] }
        set { self[ShowMorningCheckinKey.self] = newValue }
    }
    var showEveningCheckin: () -> Void {
        get { self[ShowEveningCheckinKey.self] }
        set { self[ShowEveningCheckinKey.self] = newValue }
    }
}

// MARK: - DuePrompt.synthetic helper

extension DuePrompt {
    /// Creates a synthetic DuePrompt for use with the manual-trigger snooze path.
    static func synthetic(_ id: PromptID) -> DuePrompt? {
        DuePrompt(id: id, name: id.rawValue == 2 ? "morning check-in" : "evening check-in")
    }
}
