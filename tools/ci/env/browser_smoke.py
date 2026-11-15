"""Smoke the image's two real browser executables using an isolated display."""

from playwright.sync_api import sync_playwright


def main():
    print('browser smoke: starting Playwright driver', flush=True)
    with sync_playwright() as playwright:
        for name in ('chromium', 'firefox'):
            print(name + ': launching', flush=True)
            browser_type = getattr(playwright, name)
            options = {'headless': False, 'timeout': 30000}
            if name == 'chromium':
                options['args'] = ['--no-sandbox']
            browser = browser_type.launch(**options)
            try:
                print(name + ': rendering', flush=True)
                page = browser.new_page()
                page.set_default_timeout(30000)
                page.set_content('<title>Aego CI environment</title><p>ready</p>', timeout=30000)
                if page.title() != 'Aego CI environment' or page.locator('p').inner_text() != 'ready':
                    raise RuntimeError(name + ' did not render the smoke page')
                print(name + ': browser smoke passed', flush=True)
            finally:
                print(name + ': closing', flush=True)
                browser.close()
    print('browser smoke: driver closed', flush=True)


if __name__ == '__main__':
    main()
