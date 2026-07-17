cask "daily-progress-logger" do
  version "0.1.0"
  sha256 :no_check

  url "https://github.com/cristim/daily-progress-logger/releases/download/v#{version}/DailyProgressLogger-#{version}.dmg"
  name "Daily Progress Logger"
  desc "Menu-bar app that prompts for daily plans and progress notes"
  homepage "https://github.com/cristim/daily-progress-logger"

  depends_on :macos

  app "DailyProgressLogger.app"

  # Ad-hoc signed, not notarised — strip the quarantine xattr so
  # Gatekeeper doesn't block first launch.
  preflight do
    system_command "/usr/bin/xattr",
                   args: ["-cr", staged_path/"DailyProgressLogger.app"]
  end

  uninstall launchctl: "com.cristim.daily-progress-logger"

  zap trash: [
    "~/Library/Application Support/DailyProgressLogger",
    "~/Library/LaunchAgents/com.cristim.daily-progress-logger.checkins.plist",
    "~/Library/LaunchAgents/com.cristim.daily-progress-logger.plist",
  ]

  caveats <<~CAVEATS
    Daily Progress Logger runs in the menu bar and prompts for morning
    and evening check-ins.

    To start automatically at login:
      make install-agent           # resident menu-bar app
      make install-checkin-agent   # launchd-scheduled check-ins only

    Config lives at:
      ~/Library/Application Support/DailyProgressLogger/config.json
  CAVEATS
end
