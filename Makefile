# Daily Progress Logger — Go + Qt (miqt) macOS desktop app.
#
# The miqt Qt bindings compile a large cgo surface; the first build takes
# several minutes, later builds hit the cache. Qt headers require C++17+,
# which Apple clang does not default to, hence CGO_CXXFLAGS.

VERSION     := 0.1.0
APP_NAME    := DailyProgressLogger
BINARY      := daily-progress-logger
BUNDLE_ID   := com.cristim.daily-progress-logger
BUILD_DIR   := build
APP_BUNDLE  := $(BUILD_DIR)/$(APP_NAME).app
PLIST         := $(HOME)/Library/LaunchAgents/$(BUNDLE_ID).plist
CHECKIN_PLIST := $(HOME)/Library/LaunchAgents/$(BUNDLE_ID).checkins.plist
MACDEPLOYQT   := /opt/homebrew/opt/qt/bin/macdeployqt

export CGO_CXXFLAGS := -std=c++20

.PHONY: build test lint run screenshot app dmg release install-agent uninstall-agent \
	install-checkin-agent uninstall-checkin-agent clean

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/$(BINARY)

test:
	go test -race -cover ./...

lint:
	golangci-lint run

run: build
	$(BUILD_DIR)/$(BINARY)

screenshot: build
	mkdir -p $(BUILD_DIR)/screenshots
	$(BUILD_DIR)/$(BINARY) -screenshot $(BUILD_DIR)/screenshots

# Assemble a .app bundle and vendor the Qt frameworks into it with
# macdeployqt, so the app survives Homebrew Qt upgrades.
app: build
	rm -rf $(APP_BUNDLE)
	mkdir -p $(APP_BUNDLE)/Contents/MacOS
	cp $(BUILD_DIR)/$(BINARY) $(APP_BUNDLE)/Contents/MacOS/$(BINARY)
	sed -e 's/@BINARY@/$(BINARY)/g' -e 's/@BUNDLE_ID@/$(BUNDLE_ID)/g' \
		-e 's/@APP_NAME@/$(APP_NAME)/g' -e 's/@VERSION@/$(VERSION)/g' \
		packaging/Info.plist.template > $(APP_BUNDLE)/Contents/Info.plist
	$(MACDEPLOYQT) $(APP_BUNDLE) -executable=$(APP_BUNDLE)/Contents/MacOS/$(BINARY)

# Package the .app into a distributable DMG.
dmg: app
	rm -rf $(BUILD_DIR)/.dmg-staging
	mkdir -p $(BUILD_DIR)/.dmg-staging
	cp -r $(APP_BUNDLE) $(BUILD_DIR)/.dmg-staging/
	ln -s /Applications $(BUILD_DIR)/.dmg-staging/Applications
	hdiutil create -volname "Daily Progress Logger" \
		-srcfolder $(BUILD_DIR)/.dmg-staging \
		-ov -format UDZO \
		$(BUILD_DIR)/$(APP_NAME)-$(VERSION).dmg
	rm -rf $(BUILD_DIR)/.dmg-staging

# Create a GitHub release and upload the DMG (requires gh CLI and a git tag).
release: dmg
	gh release create v$(VERSION) --generate-notes \
		$(BUILD_DIR)/$(APP_NAME)-$(VERSION).dmg

# Install a LaunchAgent so the app starts (hidden in the menu bar) at login.
install-agent: app
	mkdir -p $(HOME)/Library/LaunchAgents
	sed -e 's|@EXECUTABLE@|$(abspath $(APP_BUNDLE))/Contents/MacOS/$(BINARY)|g' \
		-e 's/@BUNDLE_ID@/$(BUNDLE_ID)/g' \
		packaging/launchagent.plist.template > $(PLIST)
	launchctl unload $(PLIST) 2>/dev/null || true
	launchctl load $(PLIST)
	@echo "LaunchAgent installed: $(PLIST)"

uninstall-agent:
	launchctl unload $(PLIST) 2>/dev/null || true
	rm -f $(PLIST)
	@echo "LaunchAgent removed"

# Scheduled check-ins without a resident app: launchd pops the due dialog
# at 09:30 and 17:30 (adjust the template if you change config times).
install-checkin-agent: app
	mkdir -p $(HOME)/Library/LaunchAgents
	sed -e 's|@EXECUTABLE@|$(abspath $(APP_BUNDLE))/Contents/MacOS/$(BINARY)|g' \
		-e 's/@BUNDLE_ID@/$(BUNDLE_ID)/g' \
		packaging/checkin-agent.plist.template > $(CHECKIN_PLIST)
	launchctl unload $(CHECKIN_PLIST) 2>/dev/null || true
	launchctl load $(CHECKIN_PLIST)
	@echo "Check-in LaunchAgent installed: $(CHECKIN_PLIST)"

uninstall-checkin-agent:
	launchctl unload $(CHECKIN_PLIST) 2>/dev/null || true
	rm -f $(CHECKIN_PLIST)
	@echo "Check-in LaunchAgent removed"

clean:
	rm -rf $(BUILD_DIR)
