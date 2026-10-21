import { TestBed } from '@angular/core/testing';
import { beforeEach, describe, expect, it } from 'vitest';
import { FakeVitals } from '../../testing/fake-vitals';
import { VITALS_FACTORY, VitalsService } from '../../vitals.service';
import { Welcome } from './welcome';

const NOT_MEDICAL = 'Not a medical device. These are wellness estimates for a breathing exercise, not a diagnosis.';

describe('Welcome', () => {
  let fake: FakeVitals;
  let video: HTMLVideoElement;

  beforeEach(() => {
    fake = new FakeVitals();
    video = document.createElement('video');
    TestBed.configureTestingModule({ providers: [{ provide: VITALS_FACTORY, useValue: () => fake }] });
  });

  async function render(): Promise<HTMLElement> {
    const fixture = TestBed.createComponent(Welcome);
    fixture.componentRef.setInput('video', video);
    await fixture.whenStable();
    return fixture.nativeElement as HTMLElement;
  }

  function buttons(el: HTMLElement): HTMLButtonElement[] {
    return Array.from(el.querySelectorAll('button'));
  }

  it('shows the not-a-medical-device notice and what is and is not measured', async () => {
    const el = await render();
    const text = el.textContent.replace(/\s+/g, ' ');

    expect(text).toContain(NOT_MEDICAL);
    expect(text).toMatch(/heart rate/i);
    expect(text).toMatch(/breathing/i);
    expect(text).toMatch(/no recording/i);
    expect(text).toMatch(/nothing leaves this device/i);
    expect(text).toMatch(/no identification/i);
  });

  it('starts the camera only when "Turn on camera" is clicked', async () => {
    const el = await render();
    expect(fake.start).not.toHaveBeenCalled();

    const all = buttons(el);
    expect(all.map((b) => b.textContent.trim())).toEqual(['Turn on camera']);
    all[0]?.click();

    expect(fake.start).toHaveBeenCalledTimes(1);
    expect(fake.start).toHaveBeenCalledWith({ video });
  });

  it('shows the last error so the visitor knows why the camera stopped', async () => {
    TestBed.inject(VitalsService);
    fake.emit('error', { code: 'camera_denied' });

    const el = await render();

    expect(el.querySelector('[role="alert"]')?.textContent).toMatch(/camera/i);
  });
});
