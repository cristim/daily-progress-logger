import Foundation
import UserNotifications

// MARK: - Local notifications (v1: static calendar triggers)
// Schedules daily reminders at the configured check-in times.
// v2: smart suppression via BGTaskScheduler (deferred — see plan).

enum Notifications {
    static let center = UNUserNotificationCenter.current()

    /// Request notification authorization and schedule triggers from config.
    /// Only call when the user enables the notify toggle in Settings.
    static func requestAndSchedule(config: CoreConfig) async {
        let granted = (try? await center.requestAuthorization(options: [.alert, .sound, .badge])) ?? false
        guard granted else { return }
        schedule(config: config)
    }

    /// Schedule repeating UNCalendarNotificationTrigger reminders from config.
    /// Replaces any previously scheduled reminders.
    static func schedule(config: CoreConfig) {
        center.removeAllPendingNotificationRequests()

        let morningComponents = timeComponents(from: config.morningTime ?? "09:30")
        let eveningComponents = timeComponents(from: config.eveningTime ?? "17:30")

        scheduleDaily(id: "morning", components: morningComponents,
                      title: "Daily Progress", body: "Time for your morning check-in.",
                      promptID: 2)
        scheduleDaily(id: "evening", components: eveningComponents,
                      title: "Daily Progress", body: "Time for your evening check-in.",
                      promptID: 3)

        if let summaryDay = config.summaryDay,
           let summaryComponents = weeklyComponents(
               day: summaryDay,
               time: config.summaryTime ?? "17:00"
           ) {
            scheduleWeekly(id: "weekly_summary", components: summaryComponents,
                           title: "Daily Progress", body: "Weekly summary ready.",
                           promptID: 4)
        }
    }

    /// Remove all pending notification requests.
    static func cancel() {
        center.removeAllPendingNotificationRequests()
    }

    // MARK: Private helpers

    private static func scheduleDaily(id: String, components: DateComponents,
                                      title: String, body: String, promptID: Int) {
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.userInfo = ["prompt_id": promptID]
        let trigger = UNCalendarNotificationTrigger(dateMatching: components, repeats: true)
        let request = UNNotificationRequest(identifier: id, content: content, trigger: trigger)
        center.add(request, withCompletionHandler: nil)
    }

    private static func scheduleWeekly(id: String, components: DateComponents,
                                       title: String, body: String, promptID: Int) {
        scheduleDaily(id: id, components: components, title: title, body: body, promptID: promptID)
    }

    private static func timeComponents(from hhmm: String) -> DateComponents {
        let parts = hhmm.split(separator: ":")
        var c = DateComponents()
        c.hour = parts.count > 0 ? Int(parts[0]) : 9
        c.minute = parts.count > 1 ? Int(parts[1]) : 30
        return c
    }

    private static func weeklyComponents(day: String, time: String) -> DateComponents? {
        let weekdayMap: [String: Int] = [
            "Sunday": 1, "Monday": 2, "Tuesday": 3, "Wednesday": 4,
            "Thursday": 5, "Friday": 6, "Saturday": 7
        ]
        guard let weekday = weekdayMap[day] else { return nil }
        var c = timeComponents(from: time)
        c.weekday = weekday
        return c
    }
}
