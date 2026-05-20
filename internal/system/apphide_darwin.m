#import <Cocoa/Cocoa.h>
#include "apphide_darwin.h"

extern void resultvAppHideDidHideCallback(void);

static id resultvAppHideObserver = nil;

void resultvStartAppHideObserver(void) {
  if (resultvAppHideObserver != nil) {
    return;
  }
  resultvAppHideObserver = [[NSNotificationCenter defaultCenter]
      addObserverForName:NSApplicationDidHideNotification
                  object:nil
                   queue:[NSOperationQueue mainQueue]
              usingBlock:^(__unused NSNotification *note) {
                resultvAppHideDidHideCallback();
              }];
}

void resultvStopAppHideObserver(void) {
  if (resultvAppHideObserver != nil) {
    [[NSNotificationCenter defaultCenter] removeObserver:resultvAppHideObserver];
    resultvAppHideObserver = nil;
  }
}
