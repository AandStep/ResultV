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

// DockMenuTarget hosts the dock menu actions. Routed through a dedicated
// NSObject (rather than swizzling custom selectors onto the Wails AppDelegate)
// so NSMenu autovalidation has a definite, unambiguous answer from
// respondsToSelector:.
@interface ResultVDockMenuTarget : NSObject
- (void)resultvDockShowAction:(id)sender;
- (void)resultvDockQuitAction:(id)sender;
@end

@implementation ResultVDockMenuTarget
- (void)resultvDockShowAction:(__unused id)sender {
  resultvDockShowGoCallback();
}
- (void)resultvDockQuitAction:(__unused id)sender {
  resultvDockQuitGoCallback();
  // Route through Cocoa's standard terminate: so the swizzled
  // applicationShouldTerminate: path runs (replyToApplicationShouldTerminate:
  // + graceful Wails BeforeClose).
  [NSApp terminate:nil];
}
@end

static BOOL gDockMenuInstalled = NO;
static id gDockQuitFinishLaunchingObserver = nil;
static ResultVDockMenuTarget *gDockMenuTarget = nil;
// Cached menu lives in a static strong reference. Building it on every right-
// click and returning a local was unreliable: Dock seemed to free the menu
// before showing it, leaving the user with only the system "Force Quit"
// fallback. With ARC, a strong static reference keeps the menu alive forever.
static NSMenu *gDockMenu = nil;

static NSMenu *resultvBuildDockMenuIfNeeded(void) {
  if (gDockMenu != nil) {
    return gDockMenu;
  }
  if (gDockMenuTarget == nil) {
    gDockMenuTarget = [[ResultVDockMenuTarget alloc] init];
  }

  NSMenu *menu = [[NSMenu alloc] initWithTitle:@""];
  [menu setAutoenablesItems:NO];

  NSMenuItem *showItem = [[NSMenuItem alloc]
      initWithTitle:@"Показать ResultV"
             action:@selector(resultvDockShowAction:)
      keyEquivalent:@""];
  [showItem setTarget:gDockMenuTarget];
  [showItem setEnabled:YES];
  [menu addItem:showItem];

  [menu addItem:[NSMenuItem separatorItem]];

  NSMenuItem *quitItem = [[NSMenuItem alloc]
      initWithTitle:@"Завершить ResultV"
             action:@selector(resultvDockQuitAction:)
      keyEquivalent:@""];
  [quitItem setTarget:gDockMenuTarget];
  [quitItem setEnabled:YES];
  [menu addItem:quitItem];

  gDockMenu = menu;
  return gDockMenu;
}

static NSMenu *resultvApplicationDockMenu(id self, SEL _cmd, NSApplication *app) {
  (void)self;
  (void)_cmd;
  (void)app;
  resultvDockMenuRequestedGoCallback();
  return resultvBuildDockMenuIfNeeded();
}

// Force-install applicationDockMenu: on the delegate's class. class_addMethod
// silently no-ops if the selector already exists on the class hierarchy. Use
// class_replaceMethod to guarantee the IMP we want.
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
  // Pre-build the menu so the first applicationDockMenu: call returns the same
  // cached instance from a stable static slot.
  resultvBuildDockMenuIfNeeded();
  gDockMenuInstalled = YES;
}

static void resultvScheduleDockMenuOnFinishLaunching(void) {
  if (gDockQuitFinishLaunchingObserver != nil) {
    return;
  }
  void (^install)(NSNotification *) = ^(__unused NSNotification *note) {
    id delegate = [NSApp delegate];
    if (delegate != nil) {
      resultvForceInstallDockMenu([delegate class]);
      resultvBuildDockMenuIfNeeded();
      gDockMenuInstalled = YES;
    }
  };
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
