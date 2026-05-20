// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>
#include "terminate_darwin.h"

// Implemented in terminate_darwin.go
extern void resultvTerminateGoCallback(void);

static BOOL gTerminateInterceptorInstalled = NO;

void resultvReplyToApplicationShouldTerminate(void) {
  void (^reply)(void) = ^{
    if ([NSApp isRunning]) {
      [NSApp replyToApplicationShouldTerminate:YES];
    }
  };
  if ([NSThread isMainThread]) {
    reply();
  } else {
    dispatch_sync(dispatch_get_main_queue(), reply);
  }
}

static NSApplicationTerminateReply resultv_applicationShouldTerminate(id self,
                                                                      SEL _cmd,
                                                                      NSApplication *app) {
  (void)self;
  (void)_cmd;
  (void)app;

  // Do NOT call Wails' applicationShouldTerminate — it synchronously enqueues
  // processMessage("Q") on the main thread and returns NSTerminateCancel without
  // replyToApplicationShouldTerminate:, which makes Dock show only Force Quit.
  dispatch_async(dispatch_get_main_queue(), ^{
    resultvTerminateGoCallback();
  });

  resultvReplyToApplicationShouldTerminate();
  return NSTerminateCancel;
}

static void resultvInstallTerminateInterceptorOnMainThread(void) {
  if (gTerminateInterceptorInstalled) {
    return;
  }
  id delegate = [NSApp delegate];
  if (delegate == nil) {
    return;
  }
  Class cls = [delegate class];
  SEL sel = @selector(applicationShouldTerminate:);
  Method method = class_getInstanceMethod(cls, sel);
  if (method == NULL) {
    return;
  }
  method_setImplementation(method, (IMP)resultv_applicationShouldTerminate);
  gTerminateInterceptorInstalled = YES;
}

int resultvInstallTerminateInterceptorSync(void) {
  if ([NSThread isMainThread]) {
    resultvInstallTerminateInterceptorOnMainThread();
    return gTerminateInterceptorInstalled ? 1 : 0;
  }
  __block int ok = 0;
  dispatch_sync(dispatch_get_main_queue(), ^{
    resultvInstallTerminateInterceptorOnMainThread();
    ok = gTerminateInterceptorInstalled ? 1 : 0;
  });
  return ok;
}
