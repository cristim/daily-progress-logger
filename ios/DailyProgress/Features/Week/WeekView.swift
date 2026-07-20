import SwiftUI

// MARK: - WeekView

/// Week tab: inline weekly-plan editing, done-by-day summary, and "Review last week" badge.
/// Toolbar menu provides manual access to "This Week's Summary…" and "Review Last Week…".
struct WeekView: View {
    let appState: AppState
    @State private var store: WeekStore
    /// State drives toolbar menu actions; sheets wired in later commits.
    @State private var showSummary = false
    @State private var showManualReview = false
    @State private var newGoalText = ""

    init(appState: AppState) {
        self.appState = appState
        // core is guaranteed non-nil when WeekView initialises: DailyProgressApp only
        // renders RootTabView (and thus tab views) after openCore() succeeds.
        _store = State(initialValue: WeekStore(
            core: appState.core!,
            onMutation: { appState.bumpDataVersion() }  // notify sibling tabs on inline edits (I2)
        ))
    }

    var body: some View {
        NavigationStack {
            Group {
                if store.isLoading && store.plan == nil && store.summary == nil {
                    ProgressView("Loading…")
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else if store.plan == nil && store.summary == nil,
                          let errMsg = store.errorMessage {
                    ContentUnavailableView(
                        "Failed to Load",
                        systemImage: "exclamationmark.triangle",
                        description: Text(errMsg)
                    )
                } else {
                    weekContent
                }
            }
            .navigationTitle("Week")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { toolbarItems }
            .toast(store.toast)
            // Show mutation errors as alerts when content is already visible (rule 4).
            .alert("Error", isPresented: Binding(
                get: { store.plan != nil && store.errorMessage != nil },
                set: { if !$0 { store.errorMessage = nil } }
            )) {
                Button("OK", role: .cancel) { store.errorMessage = nil }
            } message: {
                if let msg = store.errorMessage { Text(msg) }
            }
            // Manual summary sheet ("This Week's Summary…")
            .sheet(isPresented: $showSummary) {
                WeeklySummarySheet(
                    store: store,
                    appState: appState,
                    presentation: .manual,
                    onComplete: { appState.bumpDataVersion() },
                    onSnooze: {},
                    onSkipOrClose: {}
                )
            }
            // Manual review sheet ("Review Last Week…")
            .sheet(isPresented: $showManualReview) {
                WeekReviewSheet(
                    store: store,
                    appState: appState,
                    presentation: .manual,
                    reviewDate: Calendar.current.date(
                        byAdding: .day, value: -7, to: Date()
                    )?.coreDate ?? Date().coreDate,
                    rollover: false,
                    onComplete: { appState.bumpDataVersion() },
                    onSnooze: {},
                    onSkipOrClose: {}
                )
            }
        }
        // Refresh when another tab mutates shared data (bumpDataVersion).
        .task(id: appState.dataVersion) {
            await store.refresh()
        }
        // Re-fetch when the user navigates to a different week.
        .task(id: store.referenceDate) {
            await store.refresh()
        }
    }

    // MARK: - Week content

    private var weekContent: some View {
        List {
            bigThingsSection
            reviewBadgeSection
            doneThisWeekSection
        }
        .listStyle(.insetGrouped)
        .refreshable { await store.refresh() }
    }

    // MARK: - Big things (weekly plan goals)

    private var bigThingsSection: some View {
        Section {
            let goals = store.plan?.goals ?? []
            // enumerated() id — duplicate goal texts are legal per plan contract.
            ForEach(Array(goals.enumerated()), id: \.offset) { index, goal in
                Toggle(goal.text, isOn: Binding(
                    get: {
                        guard let gs = store.plan?.goals,
                              gs.indices.contains(index) else { return false }
                        return gs[index].done
                    },
                    set: { done in
                        Task { await store.setGoalDone(index: index, done: done) }
                    }
                ))
                .strikethrough(goal.done)
                .foregroundStyle(goal.done ? .secondary : .primary)
            }
            // Inline add row at the bottom of the section.
            HStack {
                Image(systemName: "plus.circle.fill")
                    .foregroundStyle(.green)
                TextField("Add goal…", text: $newGoalText)
                    .onSubmit {
                        let trimmed = newGoalText.trimmingCharacters(in: .whitespacesAndNewlines)
                        guard !trimmed.isEmpty else { return }
                        newGoalText = ""
                        Task { await store.addGoals(text: trimmed) }
                    }
            }
        } header: {
            Text("Big things this week")
        }
    }

    // MARK: - Review badge (only shown when a past week is unreviewed)

    @ViewBuilder
    private var reviewBadgeSection: some View {
        if store.reviewPending {
            Section {
                Button {
                    showManualReview = true
                } label: {
                    HStack {
                        Text("Review last week")
                        Spacer()
                        Image(systemName: "exclamationmark.circle.fill")
                            .foregroundStyle(.orange)
                    }
                }
            }
        }
    }

    // MARK: - Done this week (summary, done-by-day)

    @ViewBuilder
    private var doneThisWeekSection: some View {
        let daysDone = store.summary?.doneByDay ?? []
        let total = daysDone.reduce(0) { $0 + $1.items.count }

        if daysDone.isEmpty {
            Section("Done this week") {
                Text("Nothing completed yet this week.")
                    .foregroundStyle(.secondary)
            }
        } else {
            ForEach(daysDone, id: \.date) { day in
                Section(header: Text(DateFormatting.dayHeader(from: day.date))) {
                    // Items are plain strings; id by self is fine here (duplicate
                    // completed-item text within a day is extremely unlikely).
                    ForEach(day.items, id: \.self) { item in
                        Label(item, systemImage: "checkmark")
                            .foregroundStyle(.secondary)
                    }
                }
            }
            Section(footer: Text("Total: \(total) item\(total == 1 ? "" : "s") completed this week")) {
                EmptyView()
            }
        }
    }

    // MARK: - Toolbar

    @ToolbarContentBuilder
    private var toolbarItems: some ToolbarContent {
        ToolbarItem(placement: .principal) {
            weekNavigation
        }
        ToolbarItem(placement: .primaryAction) {
            Menu {
                Button("This Week's Summary…") { showSummary = true }
                Button("Review Last Week…")   { showManualReview = true }
            } label: {
                Image(systemName: "ellipsis.circle")
            }
        }
    }

    private var weekNavigation: some View {
        HStack(spacing: 16) {
            Button { store.prevWeek() } label: {
                Image(systemName: "chevron.left")
            }

            // Label: use the week string from loaded data if available;
            // fall back to a computed label from referenceDate.
            Button { store.thisWeek() } label: {
                Text(weekLabel)
                    .font(.headline)
            }

            Button { store.nextWeek() } label: {
                Image(systemName: "chevron.right")
            }
        }
    }

    private var weekLabel: String {
        store.plan?.week
            ?? store.summary?.week
            ?? DateFormatting.isoWeekLabel(from: store.referenceDate)
    }
}
