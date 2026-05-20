// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>
#include "dockquit_darwin.h"

// Implemented in dockquit_darwin.go
extern void resultvDockQuitGoCallback(void);
extern void resultvDockShowGoCallback(void);
extern void resultvDockMenuRequestedGoCallback(void);

static BOOL gDockMenuInstalled = NO;
static id gDockQuitFinishLaunchingObserver = nil;
static id gDockMenuTarget = nil;

// DockMenuTarget hosts the "Показать ResultV" action. The "Завершить ResultV"
// item targets NSApp directly via the system terminate: selector — that path is
// already swizzled in terminate_darwin.m and goes through the graceful Wails
// BeforeClose → runShutdownTasks → os.Exit(0) sequence.
//
// Routing the Show item through a dedicated NSObject (rather than swizzling a
// custom selector onto the Wails AppDelegate) means we don't depend on
// class_addMethod succeeding against a class hierarchy we don't control —
// NSMenu's autovalidation simply asks our target whether it respondsToSelector:
// and gets a definite YES.
@interface ResultVDockMenuTarget : NSObject
- (void)resultvDockShowAction:(id)sender;
@end

@implementation ResultVDockMenuTarget
- (void)resultvDockShowAction:(__unused id)sender {
  resultvDockShowGoCallback();
}
@end

static NSMenu *resultvApplicationDockMenu(id self, SEL _cmd, NSApplication *app) {
  (void)self;
  (void)_cmd;
  (void)app;
  // Log every Dock right-click request so we can tell whether macOS is even
  // routing the request to our delegate. If this never fires, the Dock has
  // decided the process is unreachable (typical when running as a different
  // session/uid than the user-session Dock) and is showing only "Force Quit".
  resultvDockMenuRequestedGoCallback();

  if (gDockMenuTarget == nil) {
    gDockMenuTarget = [[ResultVDockMenuTarget alloc] init];
  }

  NSMenu *menu = [[NSMenu alloc] initWithTitle:@""];
  // Disable autoenable so the items render enabled even if AppKit's validation
  // chain misclassifies the targets for any reason (different session, sandbox,
  // etc.). The actions themselves are safe to call unconditionally.
  [menu setAutoenablesItems:NO];

  NSMenuItem *showItem = [[NSMenuItem alloc] initWithTitle:@"Показать ResultV"
                                                    action:@selector(resultvDockShowAction:)
                                             keyEquivalent:@""];
  [showItem setTarget:gDockMenuTarget];
  [showItem setEnabled:YES];
  [menu addItem:showItem];

  [menu addItem:[NSMenuItem separatorItem]];

  // Use NSApp's own terminate: action for Quit. This selector is guaranteed to
  // exist, validates as enabled, and triggers applicationShouldTerminate: which
  // we've already swizzled to run the graceful shutdown sequence.
  NSMenuItem *quitItem = [[NSMenuItem alloc] initWithTitle:@"Завершить ResultV"
                                                    action:@selector(terminate:)
                                             keyEquivalent:@""];
  [quitItem setTarget:NSApp];
  [quitItem setEnabled:YES];
  [menu addItem:quitItem];

  return menu;
}

// Force-install applicationDockMenu: on the delegate's class. class_addMethod
// silently no-ops if the selector already exists on the class hierarchy, which
// is exactly the trap that left users stuck with "Force Quit". Use
// class_replaceMethod which guarantees the IMP we want regardless of prior
// state.
static void resultvForceInstallDockMenu(Class cls) {
  if (cls == NULL) {
    return;
  }
  if (!class_addMethod(cls, @selector(applicationDockMenu:),
                       (IMP)resultvApplicationDockMenu, "@@:@")) {
    class_replaceMethod(cls, @selector(applicationDockMenu:),
                        (IMP)resultvApplicationDockMenu, "@@:@");
  }
}

static void resultvInstallDockQuitMenuOnMainThread(void) {
  if (gDockMenuInstalled) {
    return;
  }
  id delegate = [NSApp delegate];
  if (delegate == nil) {
    return;
  }
  resultvForceInstallDockMenu([delegate class]);
  gDockMenuInstalled = YES;
}

// Late-bound installer: re-installs the swizzle as soon as macOS finishes
// launching NSApp. At this point Wails has definitely set its delegate and the
// LaunchServices process registration is complete, so the swizzle has the
// strongest possible chance of being picked up by Dock on the very first
// right-click. Idempotent — safe to call multiple times.
static void resultvScheduleDockMenuOnFinishLaunching(void) {
  if (gDockQuitFinishLaunchingObserver != nil) {
    return;
  }
  void (^install)(NSNotification *) = ^(__unused NSNotification *note) {
    id delegate = [NSApp delegate];
    if (delegate != nil) {
      resultvForceInstallDockMenu([delegate class]);
      gDockMenuInstalled = YES;
    }
  };
  // If NSApp has already finished launching by the time we get here, the
  // notification will never fire again — run the installer right away as well.
  if ([NSApp isRunning]) {
    install(nil);
  }
  gDockQuitFinishLaunchingObserver = [[NSNotificationCenter defaultCenter]
      addObserverForName:NSApplicationDidFinishLaunchingNotification
                  object:nil
                   queue:[NSOperationQueue mainQueue]
              usingBlock:install];
}

int resultvInstallDockQuitMenuSync(void) {
  if ([NSThread isMainThread]) {
    resultvInstallDockQuitMenuOnMainThread();
    resultvScheduleDockMenuOnFinishLaunching();
    return gDockMenuInstalled ? 1 : 0;
  }
  __block BOOL ok = NO;
  dispatch_sync(dispatch_get_main_queue(), ^{
    resultvInstallDockQuitMenuOnMainThread();
    resultvScheduleDockMenuOnFinishLaunching();
    ok = gDockMenuInstalled;
  });
  return ok ? 1 : 0;
}

void resultvInstallDockQuitMenu(void) {
  if ([NSThread isMainThread]) {
    resultvInstallDockQuitMenuOnMainThread();
    resultvScheduleDockMenuOnFinishLaunching();
  } else {
    dispatch_async(dispatch_get_main_queue(), ^{
      resultvInstallDockQuitMenuOnMainThread();
      resultvScheduleDockMenuOnFinishLaunching();
    });
  }
}
