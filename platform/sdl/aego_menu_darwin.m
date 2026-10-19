#import <Cocoa/Cocoa.h>

#define AEGO_MENU_CLICKS 64
#define AEGO_MENU_DEPTH 8

@interface AegoMenu : NSMenu
@end

@implementation AegoMenu
- (BOOL)performKeyEquivalent:(NSEvent *)event {
    return NO;
}
@end

@interface AegoMenuTarget : NSObject
- (void)pick:(id)sender;
@end

static int aego_clicks[AEGO_MENU_CLICKS];
static int aego_nclicks;

@implementation AegoMenuTarget
- (void)pick:(id)sender {
    if (aego_nclicks < AEGO_MENU_CLICKS) aego_clicks[aego_nclicks++] = (int)[sender tag];
}
@end

static AegoMenuTarget *aego_target;
static NSMenu *aego_bar;
static NSMenu *aego_stack[AEGO_MENU_DEPTH];
static int aego_depth;
static int aego_skipped;

static NSString *aego_str(const char *s) {
    return s ? [NSString stringWithUTF8String:s] : @"";
}

static NSMenu *aego_top(void) {
    return aego_depth > 0 ? aego_stack[aego_depth - 1] : nil;
}

void aego_menu_begin(void) {
    @autoreleasepool {
        if (!aego_target) aego_target = [[AegoMenuTarget alloc] init];
        if (aego_bar) [aego_bar release];
        aego_bar = [[AegoMenu alloc] initWithTitle:@""];
        [aego_bar setAutoenablesItems:NO];
        aego_depth = 0;
        aego_skipped = 0;
    }
}

void aego_menu_push(const char *title) {
    @autoreleasepool {
        if (aego_depth >= AEGO_MENU_DEPTH) {
            aego_skipped++;
            return;
        }
        NSString *t = aego_str(title);
        NSMenu *m = [[AegoMenu alloc] initWithTitle:t];
        [m setAutoenablesItems:NO];
        NSMenuItem *it = [[NSMenuItem alloc] initWithTitle:t action:nil keyEquivalent:@""];
        [it setSubmenu:m];
        NSMenu *parent = aego_depth == 0 ? aego_bar : aego_top();
        [parent addItem:it];
        [it release];
        aego_stack[aego_depth++] = m;
        [m release];
    }
}

void aego_menu_pop(void) {
    if (aego_skipped > 0) {
        aego_skipped--;
        return;
    }
    if (aego_depth > 0) aego_depth--;
}

void aego_menu_item(int tag, const char *label, const char *key, unsigned long long mods, int enabled, int checked) {
    @autoreleasepool {
        NSMenu *m = aego_top();
        if (!m || aego_skipped > 0) return;
        NSMenuItem *it = [[NSMenuItem alloc] initWithTitle:aego_str(label) action:@selector(pick:) keyEquivalent:aego_str(key)];
        [it setKeyEquivalentModifierMask:(NSEventModifierFlags)mods];
        [it setTarget:aego_target];
        [it setTag:tag];
        [it setEnabled:enabled ? YES : NO];
        [it setState:checked ? NSControlStateValueOn : NSControlStateValueOff];
        [m addItem:it];
        [it release];
    }
}

void aego_menu_separator(void) {
    @autoreleasepool {
        NSMenu *m = aego_top();
        if (m && aego_skipped == 0) [m addItem:[NSMenuItem separatorItem]];
    }
}

void aego_menu_role(int role, const char *label) {
    @autoreleasepool {
        NSMenu *m = aego_top();
        if (!m || aego_skipped > 0) return;
        SEL sel = NULL;
        NSString *key = @"";
        NSEventModifierFlags mods = NSEventModifierFlagCommand;
        switch (role) {
        case 1: sel = @selector(hide:); key = @"h"; break;
        case 2: sel = @selector(hideOtherApplications:); key = @"h"; mods |= NSEventModifierFlagOption; break;
        case 3: sel = @selector(unhideAllApplications:); break;
        default: return;
        }
        NSMenuItem *it = [[NSMenuItem alloc] initWithTitle:aego_str(label) action:sel keyEquivalent:key];
        [it setKeyEquivalentModifierMask:mods];
        [it setTarget:NSApp];
        [it setEnabled:YES];
        [m addItem:it];
        [it release];
    }
}

void aego_menu_commit(void) {
    @autoreleasepool {
        if (aego_bar) [NSApp setMainMenu:aego_bar];
    }
}

int aego_menu_take(int *out, int max) {
    int n = aego_nclicks < max ? aego_nclicks : max;
    for (int i = 0; i < n; i++) out[i] = aego_clicks[i];
    aego_nclicks = 0;
    return n;
}
