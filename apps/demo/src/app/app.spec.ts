import { TestBed, type ComponentFixture } from '@angular/core/testing';
import { beforeEach, describe, expect, it } from 'vitest';
import { App } from './app';
import { FakeVitals, estimate } from './testing/fake-vitals';
import { VITALS_FACTORY, VitalsService } from './vitals.service';

describe('App', () => {
  let fake: FakeVitals;
  let fixture: ComponentFixture<App>;
  let el: HTMLElement;

  beforeEach(async () => {
    fake = new FakeVitals();
    TestBed.configureTestingModule({ providers: [{ provide: VITALS_FACTORY, useValue: () => fake }] });
    fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    el = fixture.nativeElement as HTMLElement;
  });

  function button(text: string): HTMLButtonElement {
    const found = Array.from(el.querySelectorAll('button')).find((b) => b.textContent.trim() === text);
    if (found === undefined) throw new Error(`no "${text}" button`);
    return found;
  }

  it('opens on the welcome screen without touching the camera', () => {
    expect(el.querySelector('app-welcome')).not.toBeNull();
    expect(fake.start).not.toHaveBeenCalled();
    expect(el.querySelector('[data-testid="camera-indicator"]')?.textContent).toMatch(/camera off/i);
  });

  it('passes its own video element to the module on consent', async () => {
    button('Turn on camera').click();
    await fixture.whenStable();

    expect(fake.start).toHaveBeenCalledWith({ video: el.querySelector('video') });
    expect(el.querySelector('[data-testid="camera-indicator"]')?.textContent).toMatch(/camera on/i);
    expect(el.querySelector('app-welcome')).toBeNull();
  });

  it('keeps Exit visible on every screen and Exit returns to welcome', async () => {
    const service = TestBed.inject(VitalsService);
    expect(button('Exit')).toBeTruthy();

    await service.consentAndStart(document.createElement('video'));
    fake.emit('estimate', estimate());
    await fixture.whenStable();
    expect(service.screen()).toBe('exercise');

    button('Exit').click();
    await fixture.whenStable();

    expect(fake.stop).toHaveBeenCalled();
    expect(el.querySelector('app-welcome')).not.toBeNull();
    expect(el.querySelector('[data-testid="camera-indicator"]')?.textContent).toMatch(/camera off/i);
  });
});
