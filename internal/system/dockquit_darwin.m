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

static BOOL gDockMenuInstalled = NO;

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

static void resultvInstallDockQuitMenuOnMainThread(void) {
  if (gDockMenuInstalled) {
    return;
  }
  id delegate = [NSApp delegate];
  if (delegate == nil) {
    return;
  }
  Class cls = [delegate class];

  // Always replace applicationDockMenu: — class_addMethod is a no-op when the
  // selector already exists (e.g. on a Wails/AppKit delegate), leaving macOS
  // with the default unresponsive-only Dock menu.
  Method dockMenuMethod =
      class_getInstanceMethod(cls, @selector(applicationDockMenu:));
  if (dockMenuMethod != NULL) {
    method_setImplementation(dockMenuMethod, (IMP)resultvApplicationDockMenu);
  } else {
    class_addMethod(cls, @selector(applicationDockMenu:),
                    (IMP)resultvApplicationDockMenu, "@@:@");
  }
  if (class_getInstanceMethod(cls, @selector(resultvDockQuitAction:)) == NULL) {
    class_addMethod(cls, @selector(resultvDockQuitAction:),
                    (IMP)resultvDockQuitAction, "v@:@");
  }
  if (class_getInstanceMethod(cls, @selector(resultvDockShowAction:)) == NULL) {
    class_addMethod(cls, @selector(resultvDockShowAction:),
                    (IMP)resultvDockShowAction, "v@:@");
  }

  gDockMenuInstalled = YES;
}

int resultvInstallDockQuitMenuSync(void) {
  if ([NSThread isMainThread]) {
    resultvInstallDockQuitMenuOnMainThread();
    return gDockMenuInstalled ? 1 : 0;
  }
  __block BOOL ok = NO;
  dispatch_sync(dispatch_get_main_queue(), ^{
    resultvInstallDockQuitMenuOnMainThread();
    ok = gDockMenuInstalled;
  });
  return ok ? 1 : 0;
}

void resultvInstallDockQuitMenu(void) {
  if ([NSThread isMainThread]) {
    resultvInstallDockQuitMenuOnMainThread();
  } else {
    dispatch_async(dispatch_get_main_queue(), ^{
      resultvInstallDockQuitMenuOnMainThread();
    });
  }
}
