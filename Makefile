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
	go build -ldflags "-X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY) ./cmd/$(BINARY)

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
#
# macdeployqt notes (Homebrew Qt on Apple Silicon):
#   * It speculatively deploys optional plugins this app never uses (SVG,
#     PDF, virtual keyboard) whose frameworks it cannot resolve; it prints
#     "Cannot resolve rpath ..." warnings but still exits 0. We strip those
#     broken/unused components below so the shipped bundle stays clean.
#   * Its own ad-hoc code signing leaves invalid signatures on the dylibs it
#     rewrites, so the bundle fails to launch on Apple Silicon. We pass
#     -no-codesign and re-sign the finished bundle ourselves, then verify.
app:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY) ./cmd/$(BINARY)
	rm -rf $(APP_BUNDLE)
	mkdir -p $(APP_BUNDLE)/Contents/MacOS
	cp $(BUILD_DIR)/$(BINARY) $(APP_BUNDLE)/Contents/MacOS/$(BINARY)
	sed -e 's/@BINARY@/$(BINARY)/g' -e 's/@BUNDLE_ID@/$(BUNDLE_ID)/g' \
		-e 's/@APP_NAME@/$(APP_NAME)/g' -e 's/@VERSION@/$(VERSION)/g' \
		packaging/Info.plist.template > $(APP_BUNDLE)/Contents/Info.plist
	$(MACDEPLOYQT) $(APP_BUNDLE) -executable=$(APP_BUNDLE)/Contents/MacOS/$(BINARY) -no-codesign
	rm -rf $(APP_BUNDLE)/Contents/PlugIns/platforminputcontexts \
		$(APP_BUNDLE)/Contents/PlugIns/iconengines
	rm -f $(APP_BUNDLE)/Contents/PlugIns/imageformats/libqpdf.dylib
	rm -rf $(APP_BUNDLE)/Contents/Frameworks/QtQml*.framework \
		$(APP_BUNDLE)/Contents/Frameworks/QtQuick*.framework \
		$(APP_BUNDLE)/Contents/Frameworks/QtSvg.framework \
		$(APP_BUNDLE)/Contents/Frameworks/QtPdf.framework
	codesign --force --deep --sign - $(APP_BUNDLE)
	codesign --verify --deep --strict $(APP_BUNDLE)

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

# Build the shared Go core (store + sync) into an iOS xcframework for the app.
# Requires the gomobile toolchain: go install golang.org/x/mobile/cmd/gomobile@latest
ios-core:
	gomobile bind -target=ios -o ios/Frameworks/Core.xcframework ./mobilecore
	@echo "Built ios/Frameworks/Core.xcframework"

clean:
	rm -rf $(BUILD_DIR)
