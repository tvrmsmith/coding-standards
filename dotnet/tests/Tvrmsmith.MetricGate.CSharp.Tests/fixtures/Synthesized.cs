namespace Fixtures;

// The auto-property shapes the "compiler synthesizes a body" carve-out has to get right. A partial
// property is declared twice and only the implementing half is measured, whether that half writes
// accessor bodies or stays an auto-property with an initializer, so either shape yields exactly one
// pair of rows. Half and Whole declare the promise first; Early declares its implementing half
// first, so a rule keyed on declaration order reports Early's rows on the wrong lines. A static
// auto-property on an interface is not abstract, so it is measured like any other auto-property.
public partial class Held
{
    public partial int Half { get; set; }

    public partial int Whole { get; set; }

    public partial int Early { get; set; } = 5;
}

public partial class Held
{
    private int _half;

    public partial int Half
    {
        get => _half;
        set => _half = value;
    }

    public partial int Whole { get; set; } = 3;

    public partial int Early { get; set; }
}

public interface ICounted
{
    static int Counter { get; set; }

    int Uncounted { get; set; }
}
