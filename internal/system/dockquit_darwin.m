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

static void resultvDockQuitAction(id self, SEL _cmd, id sender) {
  (void)self;
  (void)_cmd;
  (void)sender;
  resultvDockQuitGoCallback();
}

static void resultvDockShowAction(id self, SEL _cmd, id sender) {
  (void)self;
  (void)_cmd;
  (void)sender;
  resultvDockShowGoCallback();
}

static NSMenu *resultvApplicationDockMenu(id self, SEL _cmd, NSApplication *app) {
  (void)_cmd;
  (void)app;
  // Log every Dock right-click request so we can tell whether macOS is even
  // routing the request to our delegate. If this never fires, the Dock has
  // decided the process is unreachable (typical when running as a different
  // session/uid than the user-session Dock) and is showing only "Force Quit".
  resultvDockMenuRequestedGoCallback();

  NSMenu *menu = [[NSMenu alloc] initWithTitle:@""];

  NSMenuItem *showItem = [[NSMenuItem alloc] initWithTitle:@"Показать ResultV"
                                                    action:@selector(resultvDockShowAction:)
                                             keyEquivalent:@""];
  [showItem setTarget:(id)self];
  [menu addItem:showItem];

  [menu addItem:[NSMenuItem separatorItem]];

  NSMenuItem *quitItem = [[NSMenuItem alloc] initWithTitle:@"Завершить ResultV"
                                                    action:@selector(resultvDockQuitAction:)
                                             keyEquivalent:@""];
  [quitItem setTarget:(id)self];
  [menu addItem:quitItem];

  return menu;
}

// Force-replace applicationDockMenu: on the delegate's class. class_addMethod
// silently no-ops if the selector already exists on the class hierarchy, which
// is exactly the trap that left users stuck with "Force Quit". Use
// class_replaceMethod which guarantees the IMP we want regardless of prior
// state, plus class_addMethod first for safety on a fresh class.
static void resultvForceInstallDockMenu(Class cls) {
  if (cls == NULL) {
    return;
  }
  if (!class_addMethod(cls, @selector(applicationDockMenu:),
                       (IMP)resultvApplicationDockMenu, "@@:@")) {
    class_replaceMethod(cls, @selector(applicationDockMenu:),
                        (IMP)resultvApplicationDockMenu, "@@:@");
  }
  if (!class_addMethod(cls, @selector(resultvDockQuitAction:),
                       (IMP)resultvDockQuitAction, "v@:@")) {
    class_replaceMethod(cls, @selector(resultvDockQuitAction:),
                        (IMP)resultvDockQuitAction, "v@:@");
  }
  if (!class_addMethod(cls, @selector(resultvDockShowAction:),
                       (IMP)resultvDockShowAction, "v@:@")) {
    class_replaceMethod(cls, @selector(resultvDockShowAction:),
                        (IMP)resultvDockShowAction, "v@:@");
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
